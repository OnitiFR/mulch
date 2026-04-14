// Disk replication for VM high availability.
//
// Replication uses QEMU dirty bitmaps and libvirt's pull-mode backup API to
// incrementally copy changed disk blocks from a source mulchd to a peer mulchd.
//
// Sync cycle (every replication_interval, per VM):
//
//  1. FSFreeze the guest (via QEMU guest agent, ~10ms)
//  2. Create a libvirt checkpoint (freezes the dirty bitmap)
//  3. FSThaw immediately
//  4. BackupBegin in pull mode: QEMU exposes dirty blocks on a TCP NBD server
//     (localhost, ephemeral port). Libvirt creates a scratch qcow2 in TempPath.
//  5. Read dirty extents via NBD BLOCK_STATUS, read data via NBD READ
//  6. Stream blocks to the peer over HTTP POST using custom MRPL binary protocol
//  7. Peer applies blocks with WriteAt on the raw replica file
//  8. BlockJobAbort to end the backup, delete the old checkpoint
//
// Files:
//   - replication.go        manager, scan loop, backoff, state, peer calls
//   - replication_sync.go   syncVM, NBD pull, MRPL stream, checkpoint/backup XML
//   - replication_state.go  ReplicationState struct and status constants
//   - replication_database.go  JSON persistence of per-VM replication state
//   - replication_receiver.go  peer-side: raw file management, MRPL receiver
//   - nbd_client.go         thin wrapper around digitalocean/go-nbd
package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"libvirt.org/go/libvirt"
)

const (
	// ReplicationStartupDelay is the start delay after VM state restoration
	ReplicationStartupDelay = 10 * time.Second
	// ReplicationScanInterval between VM-to-sync scans (half of replication_interval minimum)
	ReplicationScanInterval = 5 * time.Second
	// ReplicationFSFreezeTimeout is the timeout for FSFreeze operations
	ReplicationFSFreezeTimeout = 10 * time.Second
	// ReplicationMaxConsecutiveErrors before backoff
	ReplicationMaxConsecutiveErrors = 5
	// ReplicationPauseErrors pauses replication and sends alert
	ReplicationPauseErrors = 20
	// ReplicationMaxBackoffInterval is the maximum backoff interval
	ReplicationMaxBackoffInterval = 10 * time.Minute
)

// ReplicationManager manages disk replication for all VMs
type ReplicationManager struct {
	app              *App
	syncLocks        map[string]*sync.Mutex
	mu               sync.Mutex
	versionSupported bool
}

// NewReplicationManager creates a new ReplicationManager and checks
// libvirt/QEMU version support immediately.
func NewReplicationManager(app *App) *ReplicationManager {
	rm := &ReplicationManager{
		app:       app,
		syncLocks: make(map[string]*sync.Mutex),
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

	rm.app.Log.Info("replication manager started")

	for {
		rm.scanVMs()
		time.Sleep(ReplicationScanInterval)
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

	// app.Log.Infof("replication: libvirt %d.%d.%d, QEMU %d.%d.%d - version OK",
	// 	libvirtVer/1000000, (libvirtVer/1000)%1000, libvirtVer%1000,
	// 	qemuVer/1000000, (qemuVer/1000)%1000, qemuVer%1000)

	return true
}

// getSyncLock returns the per-VM mutex, creating it if needed
func (rm *ReplicationManager) getSyncLock(vmName string) *sync.Mutex {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	lock, exists := rm.syncLocks[vmName]
	if !exists {
		lock = &sync.Mutex{}
		rm.syncLocks[vmName] = lock
	}
	return lock
}

// abortStaleBackupJobs aborts any backup job left active by a previous crash
// and resets "syncing" states back to "idle" so the scan loop picks them up again.
func (rm *ReplicationManager) abortStaleBackupJobs() {
	vmNames := rm.app.VMDB.GetNames()
	for _, vmName := range vmNames {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil || vm.Config.ReplicationPeer == "" {
			continue
		}
		rm.AbortSync(vmName)

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

// scanVMs checks all VMs and starts sync goroutines as needed
func (rm *ReplicationManager) scanVMs() {
	vmNames := rm.app.VMDB.GetNames()

	for _, vmName := range vmNames {
		vm, err := rm.app.VMDB.GetByName(vmName)
		if err != nil {
			continue
		}

		if vm.Config.ReplicationPeer == "" {
			// replication disabled: clean up stale state if any
			if state := rm.app.ReplicationDB.Get(vmName.ID()); state != nil {
				rm.cleanupDisabledReplication(vmName, state)
			}
			continue
		}

		// check if it's time to sync
		state := rm.app.ReplicationDB.Get(vmName.ID())
		interval := rm.getEffectiveInterval(vm, state)

		if state != nil && state.Status == ReplicationSyncing {
			continue
		}

		if state != nil {
			// use the most recent of LastSyncTime and LastErrorTime for interval check,
			// so that backoff works even when no sync has ever succeeded
			lastActivity := state.LastSyncTime
			if state.LastErrorTime.After(lastActivity) {
				lastActivity = state.LastErrorTime
			}
			if !lastActivity.IsZero() && time.Since(lastActivity) < interval {
				continue
			}
		}

		// try to acquire the lock (non-blocking)
		lock := rm.getSyncLock(vmName.ID())
		if !lock.TryLock() {
			continue // already syncing
		}

		go func(name *VMName, v *VM) {
			defer lock.Unlock()
			rm.syncVM(name, v)
		}(vmName, vm)
	}
}

// getEffectiveInterval returns the sync interval, applying backoff if needed.
//
// When consecutive sync errors occur, the interval is progressively increased
// to avoid hammering a broken peer or flooding logs:
//   - < 5 errors: normal interval (as configured per VM)
//   - 5-19 errors: exponential backoff (interval doubled per error, capped at 10min)
//   - >= 20 errors: effectively paused, an alert is sent
//
// The counter resets to zero on the first successful sync.
func (rm *ReplicationManager) getEffectiveInterval(vm *VM, state *ReplicationState) time.Duration {
	interval := vm.Config.ReplicationInterval

	if state == nil || state.ConsecutiveErrors < ReplicationMaxConsecutiveErrors {
		return interval
	}

	if state.ConsecutiveErrors >= ReplicationPauseErrors {
		return ReplicationMaxBackoffInterval * 100 // effectively paused
	}

	// exponential backoff: double the interval for each error past the threshold
	backoff := interval
	for i := ReplicationMaxConsecutiveErrors; i < state.ConsecutiveErrors; i++ {
		backoff *= 2
		if backoff > ReplicationMaxBackoffInterval {
			backoff = ReplicationMaxBackoffInterval
			break
		}
	}
	return backoff
}

// ensureState returns the current ReplicationState, creating it if needed
func (rm *ReplicationManager) ensureState(vmName *VMName, vm *VM) *ReplicationState {
	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		state = &ReplicationState{
			VMName:   vmName.ID(),
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
			VMName: vmName.ID(),
		}
	}
	state.Status = ReplicationError
	state.LastError = errMsg
	state.LastErrorTime = time.Now()
	state.ConsecutiveErrors++

	if state.ConsecutiveErrors == ReplicationPauseErrors {
		rm.app.AlertSender.Send(&Alert{
			Type:    AlertTypeBad,
			Subject: fmt.Sprintf("Replication paused for VM %s", vmName.ID()),
			Content: fmt.Sprintf("Replication paused after %d consecutive errors. Last error: %s",
				state.ConsecutiveErrors, errMsg),
		})
	}

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
func (rm *ReplicationManager) peerPrepare(vm *VM, vmName *VMName) error {
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
			"disk_size": fmt.Sprintf("%d", vm.Config.DiskSize),
		},
		Log:     rm.app.Log,
		Libvirt: rm.app.Libvirt,
	}

	return call.Do()
}

// cleanupDisabledReplication cleans up replication state when replication has been
// disabled on a VM (replication_peer removed via redefine).
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

	err = cp.Delete(0)
	if err != nil {
		rm.app.Log.Warningf("replication %s: failed to delete old checkpoint '%s': %s",
			vmName.ID(), cpName, err)
	}
}

// ResetFullCopy marks a VM as needing a full copy on next sync
func (rm *ReplicationManager) ResetFullCopy(vmName *VMName) {
	state := rm.app.ReplicationDB.Get(vmName.ID())
	if state == nil {
		return
	}
	state.FullCopyDone = false
	state.LastCheckpointName = ""
	rm.app.ReplicationDB.Set(state)
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
