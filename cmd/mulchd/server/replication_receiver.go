package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

// ErrReplicaFileMissing is returned by ApplyBlocks when the replica .raw file
// is gone (e.g. deleted out-of-band on the receiver). The source peer must
// redo a full copy — which recreates the file through Prepare — instead of
// retrying the (now impossible) incremental sync forever.
var ErrReplicaFileMissing = errors.New("replica file missing, full copy required")

const (
	// ReplicationBlockMagic is the magic bytes at the start of a replication block stream
	ReplicationBlockMagic = "MRPL"
	// ReplicationProtocolVersion is the current version of the block stream protocol
	ReplicationProtocolVersion uint8 = 1
	// ReplicationMaxBlockSize is the maximum size of a single block in the stream (2 MB)
	ReplicationMaxBlockSize = 2 * 1024 * 1024
	// ReplicationEndSentinel marks the end of the block stream
	ReplicationEndSentinel uint64 = 0xFFFFFFFFFFFFFFFF
)

// ReplicationReceiver manages raw replica files on the peer side
type ReplicationReceiver struct {
	app         *App
	replicaPath string
	fileLocks   map[string]*sync.Mutex
	mu          sync.Mutex
}

// NewReplicationReceiver creates a new ReplicationReceiver and ensures the replicas directory exists
func NewReplicationReceiver(app *App) (*ReplicationReceiver, error) {
	replicaPath := path.Clean(app.Config.StoragePath + "/replicas")

	if err := os.MkdirAll(replicaPath, 0750); err != nil {
		return nil, fmt.Errorf("can't create replicas directory '%s': %s", replicaPath, err)
	}

	return &ReplicationReceiver{
		app:         app,
		replicaPath: replicaPath,
		fileLocks:   make(map[string]*sync.Mutex),
	}, nil
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
	return path.Clean(r.replicaPath + "/" + vmName + ".raw")
}

// Prepare creates or recreates a raw sparse file for the given VM and records
// the replica in the database (origin = source identity, config = raw VM TOML).
func (r *ReplicationReceiver) Prepare(vmName string, diskSize uint64, origin string, config string) error {
	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

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

	if err := r.recordReplica(vmName, origin, config, diskSize, 0); err != nil {
		return err
	}

	r.app.Log.Infof("replication receiver: prepared replica '%s' (%d bytes) from '%s'", vmName, diskSize, origin)
	return nil
}

// recordReplica creates or updates the replica database entry for a VM.
// syncBytes is only updated when > 0 (Prepare passes 0 to keep the previous value).
func (r *ReplicationReceiver) recordReplica(vmName string, origin string, config string, diskSize uint64, syncBytes uint64) error {
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

	if err := r.app.ReplicaDB.Set(state); err != nil {
		return fmt.Errorf("can't save replica database entry for '%s': %s", vmName, err)
	}
	return nil
}

// ApplyBlocks reads a binary block stream and applies writes to the replica file.
//
// Stream format (all big-endian):
//
//	Header:  "MRPL" (4 bytes) + version (uint8) + diskSize (uint64)
//	Blocks:  offset (uint64) + length (uint32) + data (length bytes) — repeated
//	End:     offset=0xFFFFFFFFFFFFFFFF + length=0
func (r *ReplicationReceiver) ApplyBlocks(vmName string, origin string, config string, body io.Reader) error {
	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

	filePath := r.filePath(vmName)

	f, err := os.OpenFile(filePath, os.O_RDWR, 0600)
	if err != nil {
		if os.IsNotExist(err) {
			// the file vanished (manual deletion, lost storage…): the source
			// can't apply an incremental stream against a missing file, signal
			// it to redo a full copy (which recreates the file via Prepare).
			return ErrReplicaFileMissing
		}
		return fmt.Errorf("can't open replica file '%s': %s", filePath, err)
	}
	defer f.Close()

	// read header
	var magic [4]byte
	if _, err := io.ReadFull(body, magic[:]); err != nil {
		return fmt.Errorf("reading magic: %s", err)
	}
	if string(magic[:]) != ReplicationBlockMagic {
		return fmt.Errorf("invalid magic: %q", magic)
	}

	var version uint8
	if err := binary.Read(body, binary.BigEndian, &version); err != nil {
		return fmt.Errorf("reading version: %s", err)
	}
	if version != ReplicationProtocolVersion {
		return fmt.Errorf("unsupported protocol version %d", version)
	}

	var diskSize uint64
	if err := binary.Read(body, binary.BigEndian, &diskSize); err != nil {
		return fmt.Errorf("reading disk size: %s", err)
	}

	// sanity check: diskSize must match the existing replica file
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("can't stat replica file: %s", err)
	}
	if uint64(fi.Size()) != diskSize {
		return fmt.Errorf("disk size mismatch: stream says %d but replica file is %d bytes", diskSize, fi.Size())
	}

	r.app.Log.Tracef("replication receiver: sync '%s' started (protocol v%d, disk size=%d)", vmName, version, diskSize)

	// read and apply blocks
	buf := make([]byte, ReplicationMaxBlockSize)
	var totalBytes uint64
	var blockCount uint64

	for {
		var offset uint64
		var length uint32

		if err := binary.Read(body, binary.BigEndian, &offset); err != nil {
			return fmt.Errorf("reading block offset: %s", err)
		}
		if err := binary.Read(body, binary.BigEndian, &length); err != nil {
			return fmt.Errorf("reading block length: %s", err)
		}

		// end sentinel
		if offset == ReplicationEndSentinel && length == 0 {
			break
		}

		if length > ReplicationMaxBlockSize {
			return fmt.Errorf("block too large: %d bytes (max %d)", length, ReplicationMaxBlockSize)
		}

		if offset+uint64(length) > diskSize {
			return fmt.Errorf("block at offset %d length %d exceeds disk size %d", offset, length, diskSize)
		}

		if _, err := io.ReadFull(body, buf[:length]); err != nil {
			return fmt.Errorf("reading block data at offset %d: %s", offset, err)
		}

		if _, err := f.WriteAt(buf[:length], int64(offset)); err != nil {
			return fmt.Errorf("writing block at offset %d: %s", offset, err)
		}

		totalBytes += uint64(length)
		if totalBytes > diskSize {
			return fmt.Errorf("total stream data (%d bytes) exceeds disk size (%d bytes)", totalBytes, diskSize)
		}
		blockCount++
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing replica file: %s", err)
	}

	if err := r.recordReplica(vmName, origin, config, diskSize, totalBytes); err != nil {
		return err
	}

	r.app.Log.Tracef("replication receiver: applied %d blocks (%d bytes) to '%s'",
		blockCount, totalBytes, vmName)
	return nil
}

// Delete removes the replica file for the given VM
func (r *ReplicationReceiver) Delete(vmName string) error {
	lock := r.getFileLock(vmName)
	lock.Lock()
	defer lock.Unlock()

	filePath := r.filePath(vmName)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("can't remove replica file '%s': %s", filePath, err)
	}

	if err := r.app.ReplicaDB.Delete(vmName); err != nil {
		return fmt.Errorf("can't remove replica database entry for '%s': %s", vmName, err)
	}

	r.app.Log.Infof("replication receiver: deleted replica '%s'", vmName)
	return nil
}
