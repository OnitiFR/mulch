package server

import "time"

// ReplicationStatus represents the current state of a VM's replication
type ReplicationStatus string

const (
	// ReplicationIdle means replication is configured but no sync is running
	ReplicationIdle ReplicationStatus = "idle"
	// ReplicationSyncing means a sync (full or incremental) is in progress
	ReplicationSyncing ReplicationStatus = "syncing"
	// ReplicationError means the last sync failed
	ReplicationError ReplicationStatus = "error"
)

// ReplicationState stores the replication state for a single VM
type ReplicationState struct {
	Name               string
	Revision           int
	PeerName           string
	Status             ReplicationStatus
	LastCheckpointName string // empty = full copy not yet done
	LastSyncTime       time.Time
	LastSyncDuration   time.Duration
	LastSyncBytes      uint64
	LastError          string
	LastErrorTime      time.Time
	FullCopyDone       bool
	ConsecutiveErrors  int
	ErrorStreakStart   time.Time // start of the current failure streak (zero = none)
	Alerted            bool      // true once an alert was sent for the current incident
}

// ID returns the unique VM ID (name + revision) for this state
func (s *ReplicationState) ID() string {
	return NewVMName(s.Name, s.Revision).ID()
}
