package common

import "time"

// APIReplicationEntries is a list of entries for "replication list"
type APIReplicationEntries []APIReplicationEntry

// APIReplicationEntry is an entry for a replicated VM (source side)
type APIReplicationEntry struct {
	Name              string
	Revision          int
	Active            bool
	PeerName          string
	Status            string
	FullCopyDone      bool
	LastSyncTime      time.Time
	LastSyncDuration  time.Duration
	LastSyncBytes     uint64
	LastError         string
	LastErrorTime     time.Time
	ConsecutiveErrors int
}
