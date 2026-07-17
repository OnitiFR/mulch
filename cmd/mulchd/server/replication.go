// Disk replication for VM high availability.
//
// Replication uses QEMU dirty bitmaps and libvirt's pull-mode backup API to
// incrementally copy changed disk blocks from a source mulchd to a peer mulchd.
//
// Sync cycle (every replication_interval, per VM):
//
//  1. FSFreeze the guest (via QEMU guest agent)
//  2. Create a libvirt checkpoint (freezes the dirty bitmap)
//  3. FSThaw immediately
//  4. BackupBegin in pull mode: QEMU exposes dirty blocks on a TCP NBD server
//     (localhost, ephemeral port). Libvirt creates a scratch qcow2 in TempPath.
//  5. Read dirty extents via NBD BLOCK_STATUS, read data via NBD READ
//  6. Stream blocks to the peer over HTTP POST using custom MRPL binary protocol
//  7. Peer applies blocks with WriteAt on the raw replica file
//  8. BlockJobAbort to end the backup, then prune all checkpoints but the new one
//
// Scheduling: a reconcile loop (every 5s) compares VMDB with running
// goroutines and spawns/stops per-VM replicator goroutines as needed.
// Each goroutine owns its sync cycle: initial jitter, syncVM, sleep(interval).
//
// Files:
//   - replication.go        manager, reconcile, per-VM goroutines, backoff, peer calls
//   - replication_sync.go   syncVM, NBD pull, MRPL stream, checkpoint/backup XML
//   - replication_state.go  ReplicationState struct and status constants
//   - replication_database.go  JSON persistence of per-VM replication state
//   - replication_receiver.go  peer-side: raw file management, MRPL receiver
//   - nbd_client.go         thin wrapper around digitalocean/go-nbd
package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
	"libvirt.org/go/libvirt"
)

// ErrPeerStandDown is detected on the source side when the peer replies with
// the stand-down sentinel (the VM was promoted there): this host must stop
// serving and replicating the VM (see handleStandDown).
var ErrPeerStandDown = errors.New("peer replied stand-down (VM was promoted there)")

const (
	// ReplicationStartupDelay is the start delay after VM state restoration
	ReplicationStartupDelay = 10 * time.Second
	// ReplicationReconcileInterval between reconcile scans
	ReplicationReconcileInterval = 5 * time.Second
	// ReplicationFSFreezeWarningDelay is the delay before logging a warning when FSFreeze is slow
	ReplicationFSFreezeWarningDelay = 10 * time.Second
	// ReplicationMaxConsecutiveErrors before backoff starts
	ReplicationMaxConsecutiveErrors = 5
	// ReplicationMaxBackoffInterval is the floor of the backoff cap
	ReplicationMaxBackoffInterval = 10 * time.Minute
	// ReplicationAlertIntervalFactor multiplies replication_interval to derive
	// the staleness threshold that triggers an alert (clamped below).
	ReplicationAlertIntervalFactor = 20
	// ReplicationAlertMinDelay is the lower bound of the derived alert delay
	ReplicationAlertMinDelay = 10 * time.Minute
	// ReplicationAlertMaxDelay is the upper bound of the derived alert delay
	ReplicationAlertMaxDelay = 2 * time.Hour
)

// vmReplicator tracks a running per-VM replication goroutine
type vmReplicator struct {
	vmID     string
	cancel   context.CancelFunc
	done     chan struct{}
	trigger  chan struct{}
	nextSync time.Time // guarded by rm.mu
}

// ReplicationManager manages disk replication for all VMs
type ReplicationManager struct {
	app              *App
	replicators      map[string]*vmReplicator
	mu               sync.Mutex
	versionSupported bool
}

// NewReplicationManager creates a new ReplicationManager and checks
// libvirt/QEMU version support immediately.
func NewReplicationManager(app *App) *ReplicationManager {
	rm := &ReplicationManager{
		app:         app,
		replicators: make(map[string]*vmReplicator),
	}

	rm.versionSupported = checkReplicationVersions(app)

	return rm
}

// Run starts the replication manager main loop.
// Waits for VM state restoration to complete before starting,
// then adds a cooldown delay.
func (rm *ReplicationManager) Run() {
	if !rm.versionSupported {
		return
	}

	rm.app.VMStateDB.WaitRestore()
	time.Sleep(ReplicationStartupDelay)

	rm.cleanupOrphanScratchFiles()
	rm.abortStaleBackupJobs()
	rm.sweepOrphanReplicationEntries()
	rm.cleanupOrphanCheckpoints()

	rm.app.Log.Info("replication manager started")

	for {
		rm.reconcile()
		rm.checkAlerts()
		time.Sleep(ReplicationReconcileInterval)
	}
}

// checkReplicationVersions verifies that libvirt and QEMU are recent enough
// for checkpoint and backup APIs (libvirt >= 6.0, QEMU >= 5.2)
func checkReplicationVersions(app *App) bool {
	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		app.Log.Errorf("replication: can't check libvirt version: %s", err)
		return false
	}

	// libvirt version as major*1000000 + minor*1000 + release
	libvirtVer, err := conn.GetLibVersion()
	if err != nil {
		app.Log.Errorf("replication: can't get libvirt version: %s", err)
		return false
	}

	// QEMU version
	qemuVer, err := conn.GetVersion()
	if err != nil {
		app.Log.Errorf("replication: can't get QEMU version: %s", err)
		return false
	}

	// libvirt >= 6.0.0 (6000000)
	if libvirtVer < 6000000 {
		app.Log.Errorf("replication: libvirt %d.%d.%d is too old (need >= 6.0.0), replication disabled",
			libvirtVer/1000000, (libvirtVer/1000)%1000, libvirtVer%1000)
		return false
	}

	// QEMU >= 5.2.0 (5002000)
	if qemuVer < 5002000 {
		app.Log.Errorf("replication: QEMU %d.%d.%d is too old (need >= 5.2.0), replication disabled",
			qemuVer/1000000, (qemuVer/1000)%1000, qemuVer%1000)
		return false
	}

	return true
}

// randomJitter returns a random duration in [0, interval).
func randomJitter(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(interval)))
}

// abortStaleBackupJobs aborts any backup job left active by a previous crash,
// thaws any guest filesystem left frozen, and resets "syncing" states back to
// "idle" so replicator goroutines pick them up.
func (rm *ReplicationManager) abortStaleBackupJobs() {
	vmNames := rm.app.VMDB.GetNames()
	for _, vmName := range vmNames {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			continue
		}
		rm.AbortSync(vmName)
		rm.thawVM(vmName)

		state := rm.app.ReplicationDB.Get(vmName.ID())
		if state != nil && state.Status == ReplicationSyncing {
			rm.app.Log.Infof("replication %s: resetting stale 'syncing' status to 'idle'", vmName.ID())
			state.Status = ReplicationIdle
			rm.app.ReplicationDB.Set(state)
		}
	}
}

// cleanupOrphanScratchFiles removes scratch files left behind by a previous crash
func (rm *ReplicationManager) cleanupOrphanScratchFiles() {
	pattern := filepath.Join(rm.app.Config.TempPath, "mulch-repl-scratch-*.qcow2")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		rm.app.Log.Warningf("replication: can't glob for orphan scratch files: %s", err)
		return
	}
	for _, f := range matches {
		rm.app.Log.Infof("replication: removing orphan scratch file '%s'", f)
		os.Remove(f)
	}
}

// sweepOrphanReplicationEntries cleans up ReplicationDB entries whose VM no
// longer exists or no longer has replication enabled (crash recovery).
func (rm *ReplicationManager) sweepOrphanReplicationEntries() {
	for _, state := range rm.app.ReplicationDB.GetAll() {
		vmName := NewVMName(state.Name, state.Revision)
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			rm.app.Log.Infof("replication %s: sweeping orphan replication entry", vmName.ID())
			rm.cleanupDisabledReplication(vmName, state)
		}
	}
}

// reconcile compares desired state (VMDB) with running goroutines and
// spawns or stops replicator goroutines as needed.
func (rm *ReplicationManager) reconcile() {
	// build desired set
	desired := make(map[string]*VMName)
	for _, vmName := range rm.app.VMDB.GetNames() {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			continue
		}
		// stood down: the VM was promoted on the peer, don't replicate it
		// anymore (see handleStandDown; 'replication full-resync' re-enables)
		if state := rm.app.ReplicationDB.Get(vmName.ID()); state != nil && state.Status == ReplicationStandDown {
			continue
		}
		desired[vmName.ID()] = vmName
	}

	// phase 1: under lock, identify what to stop and what to start
	rm.mu.Lock()
	var toStop []*vmReplicator
	for id, rep := range rm.replicators {
		if _, ok := desired[id]; !ok {
			toStop = append(toStop, rep)
			delete(rm.replicators, id)
		}
	}
	var toStart []*VMName
	for id, vmName := range desired {
		if _, exists := rm.replicators[id]; !exists {
			toStart = append(toStart, vmName)
		}
	}
	rm.mu.Unlock()

	// phase 2: stop old goroutines (outside lock, may block)
	for _, rep := range toStop {
		rm.stopReplicator(rep)
	}

	// phase 3: spawn new goroutines
	if len(toStart) > 0 {
		rm.mu.Lock()
		for _, vmName := range toStart {
			if _, exists := rm.replicators[vmName.ID()]; !exists {
				rm.spawnReplicator(vmName)
			}
		}
		rm.mu.Unlock()
	}
}

// spawnReplicator creates and starts a per-VM replication goroutine.
// Must be called with rm.mu held.
func (rm *ReplicationManager) spawnReplicator(vmName *VMName) {
	ctx, cancel := context.WithCancel(context.Background())
	rep := &vmReplicator{
		vmID:    vmName.ID(),
		cancel:  cancel,
		done:    make(chan struct{}),
		trigger: make(chan struct{}, 1),
	}
	rm.replicators[vmName.ID()] = rep
	go rm.runReplicator(ctx, vmName, rep)
}

// setNextSync records the next scheduled sync time of a replicator
// (zero while a sync is in progress)
func (rm *ReplicationManager) setNextSync(rep *vmReplicator, t time.Time) {
	rm.mu.Lock()
	rep.nextSync = t
	rm.mu.Unlock()
}

// GetNextSyncTime returns the next scheduled sync time for a VM, or a zero
// time when none is scheduled (no active replicator, or sync in progress)
func (rm *ReplicationManager) GetNextSyncTime(vmID string) time.Time {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rep, ok := rm.replicators[vmID]
	if !ok {
		return time.Time{}
	}
	return rep.nextSync
}

// stopReplicator cancels a replicator goroutine and waits for it to exit.
// Must be called WITHOUT rm.mu held.
func (rm *ReplicationManager) stopReplicator(rep *vmReplicator) {
	rep.cancel()
	vmName, err := ParseVMName(rep.vmID)
	if err == nil {
		rm.AbortSync(vmName)
	}
	<-rep.done
}

// runReplicator is the per-VM replication goroutine. It handles initial jitter,
// then loops: syncVM → sleep(effectiveInterval) until cancelled.
func (rm *ReplicationManager) runReplicator(ctx context.Context, vmName *VMName, rep *vmReplicator) {
	defer close(rep.done)

	defer func() {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			state := rm.app.ReplicationDB.Get(vmName.ID())
			if state != nil {
				rm.cleanupDisabledReplication(vmName, state)
			}
		}
	}()

	vm, err := rm.app.VMDB.GetByName(vmName)
	if err != nil {
		return
	}

	// compute initial delay: respect time remaining from last sync, or jitter
	initialDelay := rm.computeInitialDelay(vm, vmName)
	rm.setNextSync(rep, time.Now().Add(initialDelay))

	select {
	case <-time.After(initialDelay):
	case <-rep.trigger:
	case <-ctx.Done():
		return
	}

	for {
		vm, err = rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			return
		}

		rm.setNextSync(rep, time.Time{})
		rm.syncVM(vmName, vm)

		state := rm.app.ReplicationDB.Get(vmName.ID())

		// the sync ended in a stand-down (VM promoted on the peer): exit, the
		// reconcile loop won't respawn this replicator
		if state != nil && state.Status == ReplicationStandDown {
			return
		}

		interval := rm.GetEffectiveInterval(vm, state)
		rm.setNextSync(rep, time.Now().Add(interval))

		select {
		case <-time.After(interval):
		case <-rep.trigger:
		case <-ctx.Done():
			return
		}
	}
}

// computeInitialDelay returns the delay before the first sync of a replicator
// goroutine. If the VM was recently synced, it waits for the remaining interval
// plus a small jitter. Otherwise it uses a full random jitter to scatter VMs.
func (rm *ReplicationManager) computeInitialDelay(vm *VM, vmName *VMName) time.Duration {
	interval := vm.Config.ReplicationInterval

	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		return randomJitter(interval)
	}

	lastActivity := state.LastSyncTime
	if state.LastErrorTime.After(lastActivity) {
		lastActivity = state.LastErrorTime
	}

	if lastActivity.IsZero() {
		return randomJitter(interval)
	}

	effectiveInterval := rm.GetEffectiveInterval(vm, state)
	elapsed := time.Since(lastActivity)
	if elapsed < effectiveInterval {
		remaining := effectiveInterval - elapsed
		return remaining + randomJitter(interval/4)
	}

	return randomJitter(interval)
}

// GetEffectiveInterval returns the sync interval, applying backoff if needed.
//
// When consecutive sync errors occur, the interval is progressively increased
// to avoid hammering a broken peer or flooding logs:
//   - < 5 errors: normal interval (as configured per VM)
//   - >= 5 errors: exponential backoff (interval doubled per error), capped at
//     max(ReplicationMaxBackoffInterval, replication_interval) so the backoff
//     never retries more often than the configured interval.
//
// The counter resets to zero on the first successful sync. There is no "pause"
// state: a broken VM keeps retrying at the capped interval until it recovers or
// an operator intervenes.
func (rm *ReplicationManager) GetEffectiveInterval(vm *VM, state *ReplicationState) time.Duration {
	interval := vm.Config.ReplicationInterval

	if state == nil || state.ConsecutiveErrors < ReplicationMaxConsecutiveErrors {
		return interval
	}

	// the cap never goes below the configured interval, otherwise "backoff"
	// would paradoxically retry more often than normal for intervals > 10min
	maxInterval := max(ReplicationMaxBackoffInterval, interval)

	// exponential backoff: double the interval for each error past the threshold
	backoff := interval
	for i := ReplicationMaxConsecutiveErrors; i < state.ConsecutiveErrors; i++ {
		backoff *= 2
		if backoff > maxInterval {
			backoff = maxInterval
			break
		}
	}
	return backoff
}

// ComputeAlertDelay returns the staleness threshold (time without a successful
// sync) that triggers an alert for a VM. It is derived from the configured
// replication_interval and clamped to [ReplicationAlertMinDelay,
// ReplicationAlertMaxDelay].
func ComputeAlertDelay(interval time.Duration) time.Duration {
	delay := time.Duration(ReplicationAlertIntervalFactor) * interval
	if delay < ReplicationAlertMinDelay {
		return ReplicationAlertMinDelay
	}
	if delay > ReplicationAlertMaxDelay {
		return ReplicationAlertMaxDelay
	}
	return delay
}

// checkAlerts scans replication states and sends a single alert per VM when it
// has had no successful sync for longer than its derived alert delay. The alert
// fires once per incident: the Alerted flag is cleared on the next successful
// sync (see syncVM), re-arming for any future outage. No reminder and no
// recovery notification are sent, to keep operator noise minimal.
func (rm *ReplicationManager) checkAlerts() {
	now := time.Now()
	for _, state := range rm.app.ReplicationDB.GetAll() {
		if state.Alerted {
			continue
		}
		// don't alert while a sync is in progress: an initial full copy of a
		// large disk can legitimately exceed the alert delay
		if state.Status == ReplicationSyncing {
			continue
		}
		// stand-down is terminal and was alerted once by handleStandDown
		if state.Status == ReplicationStandDown {
			continue
		}

		vmName := NewVMName(state.Name, state.Revision)
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			continue
		}

		// staleness reference: time of last successful sync, or the start of the
		// current failure streak for VMs that never synced successfully
		ref := state.LastSyncTime
		if ref.IsZero() {
			ref = state.ErrorStreakStart
		}
		if ref.IsZero() {
			continue
		}

		staleness := now.Sub(ref)
		if staleness <= ComputeAlertDelay(vm.Config.ReplicationInterval) {
			continue
		}

		rm.app.AlertSender.Send(&Alert{
			Type:    AlertTypeBad,
			Subject: fmt.Sprintf("Replication failing for VM %s", vmName.ID()),
			Content: fmt.Sprintf("No successful replication for %s (%d consecutive errors). Last error: %s",
				staleness.Round(time.Second), state.ConsecutiveErrors, state.LastError),
		})

		state.Alerted = true
		rm.app.ReplicationDB.Set(state)
	}
}

// ensureState returns the current ReplicationState, creating it if needed
func (rm *ReplicationManager) ensureState(vmName *VMName, vm *VM) *ReplicationState {
	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		state = &ReplicationState{
			Name:     vmName.Name,
			Revision: vmName.Revision,
			PeerName: vm.Config.ReplicationPeer,
			Status:   ReplicationIdle,
		}
	}
	state.PeerName = vm.Config.ReplicationPeer
	return state
}

// recordError records a sync error in the replication state
func (rm *ReplicationManager) recordError(vmName *VMName, errMsg string) {
	rm.app.Log.Errorf("replication %s: %s", vmName.ID(), errMsg)

	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		state = &ReplicationState{
			Name:     vmName.Name,
			Revision: vmName.Revision,
		}
	}
	state.Status = ReplicationError
	state.LastError = errMsg
	state.LastErrorTime = time.Now()
	if state.ConsecutiveErrors == 0 {
		// anchor the start of this failure streak: used to measure staleness for
		// VMs that have never completed a successful sync (no LastSyncTime yet)
		state.ErrorStreakStart = state.LastErrorTime
	}
	state.ConsecutiveErrors++

	rm.app.ReplicationDB.Set(state)
}

// peerCleanup sends a best-effort POST /replication/cleanup to a peer
func (rm *ReplicationManager) peerCleanup(peerName string, vmName *VMName) {
	peer, exists := rm.app.Config.Peers[peerName]
	if !exists {
		rm.app.Log.Warningf("replication %s: can't cleanup peer '%s' (not found in config)", vmName.ID(), peerName)
		return
	}

	call := &PeerCall{
		Peer:   peer,
		Method: "POST",
		Path:   "/replication/cleanup",
		Args: map[string]string{
			"vm_name": vmName.ID(),
		},
		Log:     rm.app.Log,
		Libvirt: rm.app.Libvirt,
	}

	if err := call.Do(); err != nil {
		rm.app.Log.Warningf("replication %s: cleanup on peer '%s' failed: %s", vmName.ID(), peerName, err)
	}
}

// peerPrepare notifies the peer to prepare for a full copy
func (rm *ReplicationManager) peerPrepare(vm *VM, vmName *VMName, actualDiskSize uint64) error {
	peer, exists := rm.app.Config.Peers[vm.Config.ReplicationPeer]
	if !exists {
		return fmt.Errorf("peer '%s' not found in configuration", vm.Config.ReplicationPeer)
	}

	call := &PeerCall{
		Peer:   peer,
		Method: "POST",
		Path:   "/replication/prepare",
		Args: map[string]string{
			"vm_name":   vmName.ID(),
			"disk_size": fmt.Sprintf("%d", actualDiskSize),
			"vm_config": vm.Config.FileContent,
		},
		// prepare is a stream route (no HTTP status to match, unlike sync):
		// the stand-down sentinel is detected in the failure message text
		MessageCallback: func(m *common.Message) error {
			if m.Type == common.MessageFailure && strings.Contains(m.Message, common.ReplicationStandDownMessage) {
				return fmt.Errorf("%w: %s", ErrPeerStandDown, m.Message)
			}
			return nil
		},
		Log:     rm.app.Log,
		Libvirt: rm.app.Libvirt,
	}

	return call.Do()
}

// handleStandDown reacts to a stand-down reply from the peer: the VM was
// promoted there, so this host must stop serving and replicating it.
//
// The VM is deactivated (all revisions, so our proxy no longer competes with
// the peer's for its domains), the local checkpoint is deleted, and the
// replication state is durably parked in "stand-down": the reconcile loop
// stops spawning a replicator, even across restarts. A single alert is sent.
// 'replication full-resync' clears the state (operator override, see
// ResetFullCopy).
func (rm *ReplicationManager) handleStandDown(vmName *VMName, vm *VM, cause string) {
	peerName := vm.Config.ReplicationPeer
	rm.app.Log.Errorf("replication %s: peer '%s' replied stand-down (VM was promoted there): deactivating the VM and stopping its replication", vmName.ID(), peerName)

	state := rm.ensureState(vmName, vm)

	// delete the local checkpoint: no incremental will ever use it
	if state.LastCheckpointName != "" {
		if dom, err := rm.app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(rm.app)); err == nil && dom != nil {
			rm.deleteCheckpoint(dom, state.LastCheckpointName, vmName)
			dom.Free()
		}
		state.LastCheckpointName = ""
	}

	state.Status = ReplicationStandDown
	state.FullCopyDone = false
	state.LastError = cause
	state.LastErrorTime = time.Now()
	rm.app.ReplicationDB.Set(state)

	// deactivate whatever revision of this name is active: the peer serves the
	// name now, none of our revisions may claim its domains anymore
	if _, err := rm.app.VMDB.GetActiveEntryByName(vmName.Name); err == nil {
		if err := rm.app.VMDB.SetActiveRevision(vmName.Name, RevisionNone); err != nil {
			rm.app.Log.Errorf("replication %s: can't deactivate VM: %s", vmName.ID(), err)
		} else {
			rm.app.Log.Warningf("replication %s: VM deactivated (now served by peer '%s')", vmName.ID(), peerName)
		}
	}

	rm.app.AlertSender.Send(&Alert{
		Type:    AlertTypeBad,
		Subject: fmt.Sprintf("VM %s was promoted on peer %s", vmName.ID(), peerName),
		Content: fmt.Sprintf("The peer refused replication (stand-down): the VM now runs there. It was deactivated locally and its replication was stopped ('replication full-resync' would re-enable it). Cause: %s", cause),
	})
}

// cleanupDisabledReplication cleans up replication state when replication has been
// disabled on a VM (replication_peer removed via redefine) or VM was deleted.
func (rm *ReplicationManager) cleanupDisabledReplication(vmName *VMName, state *ReplicationState) {
	rm.app.Log.Infof("replication %s: replication disabled, cleaning up", vmName.ID())

	// clean up replica on the peer (best-effort)
	if state.PeerName != "" {
		rm.peerCleanup(state.PeerName, vmName)
	}

	// delete the checkpoint from the domain if it exists
	if state.LastCheckpointName != "" {
		domainName := vmName.LibvirtDomainName(rm.app)
		dom, err := rm.app.Libvirt.GetDomainByName(domainName)
		if err == nil && dom != nil {
			rm.deleteCheckpoint(dom, state.LastCheckpointName, vmName)
			dom.Free()
		}
	}

	// remove the database entry
	rm.app.ReplicationDB.Delete(vmName.ID())
}

// deleteCheckpoint deletes a checkpoint from the domain
func (rm *ReplicationManager) deleteCheckpoint(dom *libvirt.Domain, cpName string, vmName *VMName) {
	cp, err := dom.CheckpointLookupByName(cpName, 0)
	if err != nil {
		return
	}
	defer cp.Free()

	rm.deleteCheckpointObj(cp, cpName, vmName)
}

// deleteCheckpointObj deletes a checkpoint, falling back to a metadata-only
// removal when the normal delete fails because the underlying dirty bitmap is
// missing or broken in the qcow2 (orphaned libvirt metadata, e.g. after a disk
// rebuild/resize that wiped the persistent bitmaps). Returns true on success.
func (rm *ReplicationManager) deleteCheckpointObj(cp *libvirt.DomainCheckpoint, name string, vmName *VMName) bool {
	err := cp.Delete(0)
	if err == nil {
		return true
	}

	// libvirt still holds the checkpoint metadata but its bitmap is gone from
	// the disk: drop the metadata so orphans stop piling up in checkpoint-list.
	if err2 := cp.Delete(libvirt.DOMAIN_CHECKPOINT_DELETE_METADATA_ONLY); err2 != nil {
		rm.app.Log.Warningf("replication %s: failed to delete checkpoint '%s': %s (metadata-only also failed: %s)",
			vmName.ID(), name, err, err2)
		return false
	}

	rm.app.Log.Warningf("replication %s: checkpoint '%s' had no usable bitmap, removed metadata only (%s)",
		vmName.ID(), name, err)
	return true
}

// pruneCheckpoints deletes all of vmName's replication checkpoints except
// keepName (the base for the next incremental); pass "" to delete them all.
// prevName, the base of the sync just completed, is only used to log its
// removal as a normal rotation rather than an unexpected leftover.
func (rm *ReplicationManager) pruneCheckpoints(dom *libvirt.Domain, vmName *VMName, keepName string, prevName string) {
	prefix := fmt.Sprintf("mulch-repl-%s-", vmName.ID())

	cps, err := dom.ListAllCheckpoints(0)
	if err != nil {
		rm.app.Log.Warningf("replication %s: can't list checkpoints for pruning: %s", vmName.ID(), err)
		return
	}

	for i := range cps {
		cp := &cps[i]
		name, err := cp.GetName()
		if err != nil || name == keepName || !strings.HasPrefix(name, prefix) {
			cp.Free()
			continue
		}
		if rm.deleteCheckpointObj(cp, name, vmName) {
			if name == prevName {
				rm.app.Log.Tracef("replication %s: rotated out previous checkpoint '%s'", vmName.ID(), name)
			} else {
				rm.app.Log.Infof("replication %s: pruned leftover checkpoint '%s'", vmName.ID(), name)
			}
		}
		cp.Free()
	}
}

// cleanupOrphanCheckpoints removes replication checkpoints left behind by failed
// syncs or crashes, keeping only the one recorded in the replication state.
// Called once at startup.
func (rm *ReplicationManager) cleanupOrphanCheckpoints() {
	for _, vmName := range rm.app.VMDB.GetNames() {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			continue
		}

		dom, err := rm.app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(rm.app))
		if err != nil || dom == nil {
			continue
		}

		keep := ""
		if state := rm.app.ReplicationDB.Get(vmName.ID()); state != nil {
			keep = state.LastCheckpointName
		}
		rm.pruneCheckpoints(dom, vmName, keep, "")
		dom.Free()
	}
}

// ResetFullCopy marks a VM as needing a full copy on next sync. It also
// clears a stand-down state (operator override through 'replication
// full-resync'): the reconcile loop then respawns a replicator — the peer
// will keep refusing syncs anyway as long as its promoted tombstone exists.
func (rm *ReplicationManager) ResetFullCopy(vmName *VMName) {
	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		return
	}
	state.FullCopyDone = false
	state.LastCheckpointName = ""
	if state.Status == ReplicationStandDown {
		rm.app.Log.Infof("replication %s: stand-down cleared by full-resync", vmName.ID())
		state.Status = ReplicationIdle
		state.LastError = ""
	}
	rm.app.ReplicationDB.Set(state)
}

// thawVM thaws the guest filesystem if the VM is running.
// Safe to call unconditionally: FSThaw is a no-op when the FS is not frozen.
func (rm *ReplicationManager) thawVM(vmName *VMName) {
	domainName := vmName.LibvirtDomainName(rm.app)

	dom, err := rm.app.Libvirt.GetDomainByName(domainName)
	if err != nil || dom == nil {
		return
	}
	defer dom.Free()

	domState, _, err := dom.GetState()
	if err != nil || domState != libvirt.DOMAIN_RUNNING {
		return
	}

	if err := dom.FSThaw(nil, 0); err != nil {
		rm.app.Log.Warningf("replication %s: startup FSThaw failed (may be normal): %s", vmName.ID(), err)
	}
}

// AbortSync aborts any running backup block job for a VM
func (rm *ReplicationManager) AbortSync(vmName *VMName) {
	domainName := vmName.LibvirtDomainName(rm.app)

	dom, err := rm.app.Libvirt.GetDomainByName(domainName)
	if err != nil || dom == nil {
		return
	}
	defer dom.Free()

	diskDev, err := getDiskTargetDev(dom)
	if err != nil {
		return
	}

	dom.BlockJobAbort(diskDev, 0)
}

// TriggerSync wakes up a sleeping replicator goroutine so it syncs immediately.
func (rm *ReplicationManager) TriggerSync(vmName *VMName) error {
	rm.mu.Lock()
	rep, ok := rm.replicators[vmName.ID()]
	rm.mu.Unlock()

	if !ok {
		return fmt.Errorf("no active replicator for VM %s", vmName.ID())
	}

	select {
	case rep.trigger <- struct{}{}:
	default:
	}
	return nil
}
