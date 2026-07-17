package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
)

var ErrReplicaFileMissing = errors.New("replica file missing, full copy required")
var ErrReplicaOriginConflict = errors.New("replica name already owned by another peer")

// ErrReplicaPromoted is the stand-down sentinel: the source peer must stop
// replicating this VM, now served by this host (see handleStandDown for the
// source-side handling).
var ErrReplicaPromoted = errors.New(common.ReplicationStandDownMessage)

const (
	// ReplicationBlockMagic is the magic bytes at the start of a replication block stream
	ReplicationBlockMagic = "MRPL"
	// ReplicationProtocolVersion is the current version of the block stream protocol
	ReplicationProtocolVersion uint8 = 1
	// ReplicationMaxBlockSize is the maximum size of a single block in the stream (2 MB)
	ReplicationMaxBlockSize = 2 * 1024 * 1024
	// ReplicationEndSentinel marks the end of the block stream
	ReplicationEndSentinel uint64 = 0xFFFFFFFFFFFFFFFF

	// replica disk file, then the two stages of an incremental sync: a stream
	// being spooled (.part), then a committed delta pending replay (.journal).
	// Leftovers of either are reconciled at boot by recoverJournals.
	replicaRawSuffix     = ".raw"
	replicaPartSuffix    = ".mrpl.part"
	replicaJournalSuffix = ".mrpl.journal"
)

// ReplicationReceiver manages raw replica files on the peer side
type ReplicationReceiver struct {
	app         *App
	replicaPath string
	fileLocks   map[string]*sync.Mutex
	mu          sync.Mutex

	syncing map[string]bool
	syncMu  sync.Mutex

	// originMu serializes origin-conflict checks so two peers can't concurrently
	// pass the check and both claim the same VM name
	originMu sync.Mutex
}

// NewReplicationReceiver creates a new ReplicationReceiver and ensures the replicas directory exists
func NewReplicationReceiver(app *App) (*ReplicationReceiver, error) {
	replicaPath := path.Clean(app.Config.StoragePath + "/replicas")

	if err := os.MkdirAll(replicaPath, 0750); err != nil {
		return nil, fmt.Errorf("can't create replicas directory '%s': %s", replicaPath, err)
	}

	r := &ReplicationReceiver{
		app:         app,
		replicaPath: replicaPath,
		fileLocks:   make(map[string]*sync.Mutex),
		syncing:     make(map[string]bool),
	}

	// reconcile any staging journal left by a previous unclean shutdown before
	// accepting syncs, so the crash-consistency invariant holds from boot.
	if err := r.recoverJournals(); err != nil {
		return nil, err
	}

	// reconcile any promote interrupted by an unclean shutdown, so the replica
	// is not left wedged in the transient "promoting" state forever.
	r.recoverPromotes()

	return r, nil
}

// recoverPromotes reconciles replicas left in the transient "promoting" state
// by a mulchd crash. Two cases, told apart by where the .raw sits:
//
//   - .raw still in replicas/: the promote died before (or rolled back) the
//     move. Re-accept syncs, but flag the replica as diverged: the guest may
//     have booted during the attempt, so only a full copy is trustworthy.
//   - .raw gone: the disk was moved to the disks pool; a VM may or may not
//     have been created before the crash. Keep refusing syncs (tombstone) and
//     let the admin sort it out: either the VM exists and all is well, or the
//     disk must be moved back by hand (then 'replica delete' the tombstone).
func (r *ReplicationReceiver) recoverPromotes() {
	for _, state := range r.app.ReplicaDB.GetAll() {
		if !state.Promoting {
			continue
		}
		vmID := state.ID()

		state.Promoting = false
		if _, err := os.Stat(r.filePath(vmID)); err == nil {
			state.Diverged = true
			r.app.Log.Warningf("replication receiver: promote of '%s' was interrupted by a shutdown; replica kept (as diverged: the source will have to redo a full copy)", vmID)
		} else {
			state.Promoted = true
			r.app.Log.Errorf("replication receiver: promote of '%s' was interrupted by a shutdown AFTER its disk moved to the disks pool; marked as promoted — check that the VM exists, otherwise move '%s.raw' back to the replicas directory and delete the tombstone", vmID, vmID)
		}

		if err := r.app.ReplicaDB.Set(state); err != nil {
			r.app.Log.Errorf("replication receiver: can't save recovered promote state for '%s': %s", vmID, err)
		}
	}
}

// markSyncing flags a VM ID as currently receiving a sync.
func (r *ReplicationReceiver) markSyncing(vmName string) {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	r.syncing[vmName] = true
}

// unmarkSyncing clears the syncing flag for a VM ID.
func (r *ReplicationReceiver) unmarkSyncing(vmName string) {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	delete(r.syncing, vmName)
}

// SyncingIDs returns a set of VM IDs currently receiving a sync.
func (r *ReplicationReceiver) SyncingIDs() map[string]bool {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	out := make(map[string]bool, len(r.syncing))
	for id := range r.syncing {
		out[id] = true
	}
	return out
}

// getFileLock returns a per-VM mutex, creating it if needed
func (r *ReplicationReceiver) getFileLock(vmName string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()

	lock, exists := r.fileLocks[vmName]
	if !exists {
		lock = &sync.Mutex{}
		r.fileLocks[vmName] = lock
	}
	return lock
}

func (r *ReplicationReceiver) filePath(vmName string) string {
	return path.Clean(r.replicaPath + "/" + vmName + replicaRawSuffix)
}

// partPath is the in-progress staging journal for an incremental sync.
func (r *ReplicationReceiver) partPath(vmName string) string {
	return path.Clean(r.replicaPath + "/" + vmName + replicaPartSuffix)
}

// journalPath is the committed staging journal pending replay onto the .raw.
func (r *ReplicationReceiver) journalPath(vmName string) string {
	return path.Clean(r.replicaPath + "/" + vmName + replicaJournalSuffix)
}

// checkOriginConflict returns ErrReplicaOriginConflict if a replica with the
// same VM name (any revision) is already owned by a different peer. This keeps
// a VM name bound to a single source: a peer must not be able to overwrite or
// shadow a replica that belongs to another origin.
func (r *ReplicationReceiver) checkOriginConflict(vmName string, origin string) error {
	name, err := ParseVMName(vmName)
	if err != nil {
		return fmt.Errorf("invalid VM name '%s': %s", vmName, err)
	}

	for _, state := range r.app.ReplicaDB.GetAllForName(name.Name) {
		if state.Origin != origin {
			return fmt.Errorf("%w: '%s' is owned by '%s', refusing replication from '%s'",
				ErrReplicaOriginConflict, name.Name, state.Origin, origin)
		}
	}
	return nil
}

// checkPromoted refuses any replication activity for a VM name that was (or is
// being) promoted on this peer, whatever the revision: the tombstone must keep
// standing even if the source rebuilds the VM under a new revision.
func (r *ReplicationReceiver) checkPromoted(vmName string) error {
	name, err := ParseVMName(vmName)
	if err != nil {
		return fmt.Errorf("invalid VM name '%s': %s", vmName, err)
	}

	for _, state := range r.app.ReplicaDB.GetAllForName(name.Name) {
		if state.Promoted || state.Promoting {
			return fmt.Errorf("%w: '%s' now runs as a local VM", ErrReplicaPromoted, name.Name)
		}
	}
	return nil
}

// Prepare creates or recreates a raw sparse file for the given VM and records
// the replica in the database (origin = source identity, config = raw VM TOML).
func (r *ReplicationReceiver) Prepare(vmName string, diskSize uint64, origin string, config string) error {
	// hold originMu across the conflict check and the ownership claim
	// (recordReplica) so two peers can't both pass the check before either
	// records its entry.
	r.originMu.Lock()
	defer r.originMu.Unlock()

	if err := r.checkOriginConflict(vmName, origin); err != nil {
		return err
	}

	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

	// checked under the file lock so a promote that just set its state can't
	// be raced by a sync that passed the check before the lock
	if err := r.checkPromoted(vmName); err != nil {
		return err
	}

	r.markSyncing(vmName)
	defer r.unmarkSyncing(vmName)

	filePath := r.filePath(vmName)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("can't create replica file '%s': %s", filePath, err)
	}
	defer f.Close()

	// sparse allocation: sets file size without allocating disk space
	if err := f.Truncate(int64(diskSize)); err != nil {
		os.Remove(filePath)
		return fmt.Errorf("can't set replica file size: %s", err)
	}

	// the file was just truncated: any staging journal from a previous
	// incremental cycle no longer applies to it and must not be replayed.
	os.Remove(r.partPath(vmName))
	os.Remove(r.journalPath(vmName))

	if err := r.recordReplica(vmName, origin, config, diskSize, 0, func(s *ReplicaState) {
		// no consistent point exists until the full copy completes
		s.ConsistentSnapshot = false
		s.Applying = false
		// the file was just truncated: any divergence is gone with it
		s.Diverged = false
	}); err != nil {
		return err
	}

	r.app.Log.Infof("replication receiver: prepared replica '%s' (%d bytes) from '%s'", vmName, diskSize, origin)
	return nil
}

// recordReplica creates or updates the replica database entry for a VM.
// syncBytes is only updated when > 0 (Prepare passes 0 to keep the previous
// value). The optional mutate callback runs on the state before it is saved,
// letting callers set extra fields (ConsistentSnapshot, Applying…) in the same
// atomic save.
func (r *ReplicationReceiver) recordReplica(vmName string, origin string, config string, diskSize uint64, syncBytes uint64, mutate func(*ReplicaState)) error {
	name, err := ParseVMName(vmName)
	if err != nil {
		return fmt.Errorf("invalid VM name '%s': %s", vmName, err)
	}

	state := r.app.ReplicaDB.Get(vmName)
	if state == nil {
		state = &ReplicaState{
			Name:     name.Name,
			Revision: name.Revision,
		}
	}
	state.Origin = origin
	state.Config = config
	state.DiskSize = diskSize
	state.LastUpdate = time.Now()
	if syncBytes > 0 {
		state.LastSyncBytes = syncBytes
	}
	if mutate != nil {
		mutate(state)
	}

	if err := r.app.ReplicaDB.Set(state); err != nil {
		return fmt.Errorf("can't save replica database entry for '%s': %s", vmName, err)
	}
	return nil
}

// setApplying updates the informational Applying flag on the replica entry (the
// authoritative signal stays the on-disk journal). It is a no-op if no entry
// exists yet.
func (r *ReplicationReceiver) setApplying(vmName string, applying bool) {
	state := r.app.ReplicaDB.Get(vmName)
	if state == nil {
		return
	}
	state.Applying = applying
	if err := r.app.ReplicaDB.Set(state); err != nil {
		r.app.Log.Errorf("replication receiver: can't update Applying flag for '%s': %s", vmName, err)
	}
}

// ApplyBlocks reads a binary block stream and applies it to the replica file.
//
// Stream format (all big-endian):
//
//	Header:  "MRPL" (4 bytes) + version (uint8) + diskSize (uint64)
//	Blocks:  offset (uint64) + length (uint32) + data (length bytes) — repeated
//	End:     offset=0xFFFFFFFFFFFFFFFF + length=0
//
// fullCopy selects the write strategy: see applyFullCopy (in place) and
// applyIncremental (staged through a journal) for the crash-consistency
// rationale.
func (r *ReplicationReceiver) ApplyBlocks(vmName string, origin string, config string, fullCopy bool, body io.Reader) error {
	r.originMu.Lock()
	conflict := r.checkOriginConflict(vmName, origin)
	r.originMu.Unlock()
	if conflict != nil {
		return conflict
	}

	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

	// see Prepare for the locking rationale
	if err := r.checkPromoted(vmName); err != nil {
		return err
	}

	r.markSyncing(vmName)
	defer r.unmarkSyncing(vmName)

	if fullCopy {
		return r.applyFullCopy(vmName, origin, config, body)
	}
	return r.applyIncremental(vmName, origin, config, body)
}

// applyFullCopy writes a full-copy stream directly onto the replica .raw.
// Prepare just truncated the file, so there is no previous consistent point to
// protect: ConsistentSnapshot stays false for the whole operation and is set to
// true only once the full copy completes successfully.
func (r *ReplicationReceiver) applyFullCopy(vmName string, origin string, config string, body io.Reader) error {
	filePath := r.filePath(vmName)

	f, err := os.OpenFile(filePath, os.O_RDWR, 0600)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrReplicaFileMissing
		}
		return fmt.Errorf("can't open replica file '%s': %s", filePath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("can't stat replica file: %s", err)
	}
	diskSize := uint64(fi.Size())

	r.app.Log.Tracef("replication receiver: full-copy sync '%s' started (disk size=%d)", vmName, diskSize)

	totalBytes, err := readMRPLStream(body, diskSize, func(offset uint64, data []byte) error {
		_, werr := f.WriteAt(data, int64(offset))
		return werr
	})
	if err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing replica file: %s", err)
	}

	if err := r.recordReplica(vmName, origin, config, diskSize, totalBytes, func(s *ReplicaState) {
		// the .raw now holds a complete, consistent image
		s.ConsistentSnapshot = true
		s.Diverged = false
	}); err != nil {
		return err
	}

	r.app.Log.Infof("replication receiver: full copy applied (%d bytes) to '%s'", totalBytes, vmName)
	return nil
}

// applyIncremental stages an incremental stream through a journal so the .raw
// is never observed in a torn state: it is spooled to a side file, atomically
// committed (rename), then replayed onto the .raw (see the numbered steps
// below). If the source crashes mid-stream the incomplete spool is discarded
// and the .raw stays at its previous consistent point.
func (r *ReplicationReceiver) applyIncremental(vmName string, origin string, config string, body io.Reader) error {
	filePath := r.filePath(vmName)
	partPath := r.partPath(vmName)
	journalPath := r.journalPath(vmName)

	// the replica .raw must already exist for an incremental stream; if it is
	// gone (manual deletion, lost storage…), signal the source to redo a full
	// copy, which recreates the file via Prepare.
	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrReplicaFileMissing
		}
		return fmt.Errorf("can't stat replica file '%s': %s", filePath, err)
	}
	diskSize := uint64(fi.Size())

	// a diverged .raw (a failed promote booted the guest, which wrote to the
	// disk) can't be fixed by deltas: force the source to redo a full copy
	if state := r.app.ReplicaDB.Get(vmName); state != nil && state.Diverged {
		return fmt.Errorf("%w (diverged by a failed promote)", ErrReplicaFileMissing)
	}

	// --- 1. spool to the staging journal (validate only, no .raw write) ---
	part, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("can't create staging journal '%s': %s", partPath, err)
	}

	// the TeeReader mirrors every byte consumed by the parser into the .part,
	// so a clean parse leaves an exact copy of the MRPL stream on disk.
	_, err = readMRPLStream(io.TeeReader(body, part), diskSize, nil)
	if err != nil {
		part.Close()
		os.Remove(partPath)
		return fmt.Errorf("staging incremental stream: %s", err)
	}

	if err := part.Sync(); err != nil {
		part.Close()
		os.Remove(partPath)
		return fmt.Errorf("syncing staging journal: %s", err)
	}
	if err := part.Close(); err != nil {
		os.Remove(partPath)
		return fmt.Errorf("closing staging journal: %s", err)
	}

	// --- 2. atomic commit: .part -> .journal ---
	if err := os.Rename(partPath, journalPath); err != nil {
		os.Remove(partPath)
		return fmt.Errorf("committing staging journal: %s", err)
	}
	r.setApplying(vmName, true)

	// --- 3. replay the committed journal onto the .raw ---
	totalBytes, err := r.replayJournal(vmName, diskSize)
	if err != nil {
		// the journal is committed and stays on disk: it will be replayed at the
		// next boot (idempotent). Surface the error so the source can retry.
		return fmt.Errorf("applying staging journal: %s", err)
	}

	if err := r.recordReplica(vmName, origin, config, diskSize, totalBytes, func(s *ReplicaState) {
		s.Applying = false
		// a fully replayed incremental leaves the .raw at a complete FSFreeze
		// point (the source only streams incrementals after a successful full
		// copy, so this can never mask a mid-full-copy torn image)
		s.ConsistentSnapshot = true
	}); err != nil {
		return err
	}

	r.app.Log.Tracef("replication receiver: applied incremental journal (%d bytes) to '%s'", totalBytes, vmName)
	return nil
}

// replayJournal applies a committed staging journal onto the replica .raw and
// removes it on success. WriteAt is idempotent, so replaying a journal that was
// already partially applied (crash mid-replay) is safe.
func (r *ReplicationReceiver) replayJournal(vmName string, diskSize uint64) (uint64, error) {
	journalPath := r.journalPath(vmName)
	filePath := r.filePath(vmName)

	jf, err := os.Open(journalPath)
	if err != nil {
		return 0, fmt.Errorf("can't open staging journal '%s': %s", journalPath, err)
	}
	defer jf.Close()

	f, err := os.OpenFile(filePath, os.O_RDWR, 0600)
	if err != nil {
		return 0, fmt.Errorf("can't open replica file '%s': %s", filePath, err)
	}
	defer f.Close()

	totalBytes, err := readMRPLStream(jf, diskSize, func(offset uint64, data []byte) error {
		_, werr := f.WriteAt(data, int64(offset))
		return werr
	})
	if err != nil {
		return totalBytes, err
	}

	if err := f.Sync(); err != nil {
		return totalBytes, fmt.Errorf("syncing replica file: %s", err)
	}

	if err := os.Remove(journalPath); err != nil {
		return totalBytes, fmt.Errorf("removing applied journal '%s': %s", journalPath, err)
	}

	return totalBytes, nil
}

// readMRPLStream parses an MRPL block stream from rd, validating the header
// against expectSize (the replica .raw size). For each data block it invokes
// apply (when non-nil) to write the block's bytes at its offset; a nil apply
// only validates the stream (used to spool through a TeeReader). It returns the
// total number of data bytes carried by the stream.
//
// The stream is only complete once the explicit end sentinel is read: a
// premature EOF (source crashed mid-copy) returns an error, so a truncated
// journal is never mistaken for a finished one.
func readMRPLStream(rd io.Reader, expectSize uint64, apply func(offset uint64, data []byte) error) (uint64, error) {
	var magic [4]byte
	if _, err := io.ReadFull(rd, magic[:]); err != nil {
		return 0, fmt.Errorf("reading magic: %s", err)
	}
	if string(magic[:]) != ReplicationBlockMagic {
		return 0, fmt.Errorf("invalid magic: %q", magic)
	}

	var version uint8
	if err := binary.Read(rd, binary.BigEndian, &version); err != nil {
		return 0, fmt.Errorf("reading version: %s", err)
	}
	if version != ReplicationProtocolVersion {
		return 0, fmt.Errorf("unsupported protocol version %d", version)
	}

	var diskSize uint64
	if err := binary.Read(rd, binary.BigEndian, &diskSize); err != nil {
		return 0, fmt.Errorf("reading disk size: %s", err)
	}
	if diskSize != expectSize {
		return 0, fmt.Errorf("disk size mismatch: stream says %d but replica file is %d bytes", diskSize, expectSize)
	}

	buf := make([]byte, ReplicationMaxBlockSize)
	var totalBytes uint64

	for {
		var offset uint64
		var length uint32

		if err := binary.Read(rd, binary.BigEndian, &offset); err != nil {
			return totalBytes, fmt.Errorf("reading block offset: %s", err)
		}
		if err := binary.Read(rd, binary.BigEndian, &length); err != nil {
			return totalBytes, fmt.Errorf("reading block length: %s", err)
		}

		// end sentinel
		if offset == ReplicationEndSentinel && length == 0 {
			break
		}

		if length > ReplicationMaxBlockSize {
			return totalBytes, fmt.Errorf("block too large: %d bytes (max %d)", length, ReplicationMaxBlockSize)
		}
		if offset+uint64(length) > diskSize {
			return totalBytes, fmt.Errorf("block at offset %d length %d exceeds disk size %d", offset, length, diskSize)
		}

		if _, err := io.ReadFull(rd, buf[:length]); err != nil {
			return totalBytes, fmt.Errorf("reading block data at offset %d: %s", offset, err)
		}

		if apply != nil {
			if err := apply(offset, buf[:length]); err != nil {
				return totalBytes, fmt.Errorf("writing block at offset %d: %s", offset, err)
			}
		}

		totalBytes += uint64(length)
		if totalBytes > diskSize {
			return totalBytes, fmt.Errorf("total stream data (%d bytes) exceeds disk size (%d bytes)", totalBytes, diskSize)
		}
	}

	return totalBytes, nil
}

// recoverJournals reconciles staging journals left by an unclean shutdown,
// restoring the crash-consistency invariant before any sync is accepted:
//
//   - a <vm>.mrpl.part (spool never committed: source crashed mid-stream) is
//     discarded; the .raw keeps its previous consistent point;
//   - a <vm>.mrpl.journal (committed but maybe not fully replayed: we crashed
//     mid-replay) is replayed onto the .raw (idempotent), then removed.
func (r *ReplicationReceiver) recoverJournals() error {
	entries, err := os.ReadDir(r.replicaPath)
	if err != nil {
		return fmt.Errorf("can't scan replicas directory '%s': %s", r.replicaPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		switch {
		case strings.HasSuffix(name, replicaPartSuffix):
			partPath := path.Clean(r.replicaPath + "/" + name)
			if err := os.Remove(partPath); err != nil {
				r.app.Log.Errorf("replication receiver: can't remove stale staging journal '%s': %s", partPath, err)
				continue
			}
			r.app.Log.Warningf("replication receiver: discarded incomplete staging journal '%s' (source crashed mid-sync); replica left at its last consistent point", name)

		case strings.HasSuffix(name, replicaJournalSuffix):
			r.recoverOneJournal(strings.TrimSuffix(name, replicaJournalSuffix))
		}
	}

	return nil
}

// recoverOneJournal replays a single committed journal at startup. On any error
// the journal is left in place (never silently dropped): a later full copy
// (Prepare) clears it, or a subsequent incremental commit overwrites it.
func (r *ReplicationReceiver) recoverOneJournal(vmName string) {
	fi, err := os.Stat(r.filePath(vmName))
	if err != nil {
		r.app.Log.Warningf("replication receiver: staging journal for '%s' has no replica file, leaving it for the next full copy to clear: %s", vmName, err)
		return
	}

	totalBytes, err := r.replayJournal(vmName, uint64(fi.Size()))
	if err != nil {
		r.app.Log.Errorf("replication receiver: can't replay staging journal for '%s' (left in place): %s", vmName, err)
		return
	}

	r.setApplying(vmName, false)
	r.app.Log.Infof("replication receiver: replayed committed staging journal for '%s' (%d bytes) after restart", vmName, totalBytes)
}

// RawFilePath returns the path of the replica .raw file for a VM ID
func (r *ReplicationReceiver) RawFilePath(vmName string) string {
	return r.filePath(vmName)
}

// BeginPromote durably switches a replica to the "promoting" state. It runs
// under the file lock, so it serializes with any in-flight sync; once it
// returns, no new prepare/sync can touch this VM name (checkPromoted) and the
// .raw is guaranteed to sit at a complete FSFreeze point: a committed journal
// left by an earlier failed replay is replayed first, and a replica without a
// consistent snapshot (full copy in progress) is refused.
func (r *ReplicationReceiver) BeginPromote(vmID string) (*ReplicaState, error) {
	lock := r.getFileLock(vmID)
	lock.Lock()
	defer lock.Unlock()

	state := r.app.ReplicaDB.Get(vmID)
	if state == nil {
		return nil, fmt.Errorf("no replica named '%s'", vmID)
	}
	if state.Promoted {
		return nil, fmt.Errorf("replica '%s' was already promoted", vmID)
	}
	if state.Promoting {
		return nil, fmt.Errorf("a promote is already in progress for '%s'", vmID)
	}
	if !state.ConsistentSnapshot {
		return nil, fmt.Errorf("replica '%s' has no consistent snapshot (full copy in progress or incomplete), refusing to promote", vmID)
	}

	fi, err := os.Stat(r.filePath(vmID))
	if err != nil {
		return nil, fmt.Errorf("can't stat replica file: %s", err)
	}

	if _, err := os.Stat(r.journalPath(vmID)); err == nil {
		// a committed journal whose replay failed earlier: replay it now
		// (idempotent) so the promote uses the latest complete FSFreeze point
		if _, err := r.replayJournal(vmID, uint64(fi.Size())); err != nil {
			return nil, fmt.Errorf("can't replay pending staging journal: %s", err)
		}
		state.Applying = false
	}

	state.Promoting = true
	if err := r.app.ReplicaDB.Set(state); err != nil {
		return nil, fmt.Errorf("can't save replica database entry: %s", err)
	}

	r.app.Log.Infof("replication receiver: replica '%s' is now promoting, incoming syncs are refused", vmID)
	return state, nil
}

// AbortPromote rolls back the "promoting" state after a failed promote, so
// the replica accepts incoming syncs again. diverged must be true if the
// guest was booted during the failed attempt (it wrote to the .raw): the
// replica then refuses incremental syncs until the source redoes a full copy
// (see ReplicaState.Diverged).
func (r *ReplicationReceiver) AbortPromote(vmID string, diverged bool) {
	lock := r.getFileLock(vmID)
	lock.Lock()
	defer lock.Unlock()

	state := r.app.ReplicaDB.Get(vmID)
	if state == nil || !state.Promoting {
		return
	}
	state.Promoting = false
	if diverged && !state.Diverged {
		state.Diverged = true
		r.app.Log.Warningf("replication receiver: replica '%s' diverged (failed promote booted the guest); still promotable, but the source will have to redo a full copy", vmID)
	}
	if err := r.app.ReplicaDB.Set(state); err != nil {
		r.app.Log.Errorf("replication receiver: can't clear promoting state for '%s': %s", vmID, err)
	}
}

// FinishPromote switches a promoting replica to its final "promoted" tombstone
// state: the .raw was moved to the disks pool by the caller, only the database
// entry remains (to keep refusing syncs from the original source, see
// checkPromoted). Staging leftovers are dropped.
func (r *ReplicationReceiver) FinishPromote(vmID string) error {
	lock := r.getFileLock(vmID)
	lock.Lock()
	defer lock.Unlock()

	os.Remove(r.partPath(vmID))
	os.Remove(r.journalPath(vmID))

	state := r.app.ReplicaDB.Get(vmID)
	if state == nil {
		return fmt.Errorf("no replica database entry for '%s'", vmID)
	}
	state.Promoting = false
	state.Promoted = true
	state.Applying = false
	state.LastUpdate = time.Now()
	if err := r.app.ReplicaDB.Set(state); err != nil {
		return fmt.Errorf("can't save replica database entry: %s", err)
	}

	r.app.Log.Infof("replication receiver: replica '%s' promoted (tombstone kept, delete it to allow replication again)", vmID)
	return nil
}

// Delete removes the replica file for the given VM
func (r *ReplicationReceiver) Delete(vmName string) error {
	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

	if state := r.app.ReplicaDB.Get(vmName); state != nil && state.Promoting {
		return fmt.Errorf("a promote is in progress for '%s'", vmName)
	}

	filePath := r.filePath(vmName)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't remove replica file '%s': %s", filePath, err)
	}

	// drop any staging journal so it can't be replayed onto a recreated replica
	os.Remove(r.partPath(vmName))
	os.Remove(r.journalPath(vmName))

	if err := r.app.ReplicaDB.Delete(vmName); err != nil {
		return fmt.Errorf("can't remove replica database entry for '%s': %s", vmName, err)
	}

	r.app.Log.Infof("replication receiver: deleted replica '%s'", vmName)
	return nil
}
