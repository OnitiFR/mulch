package common

import "time"

// APIReplicaEntries is a list of entries for "replica list"
type APIReplicaEntries []APIReplicaEntry

// APIReplicaEntry is an entry for a replica held by this peer (receiver side)
type APIReplicaEntry struct {
	Name          string
	Revision      int
	Origin        string
	DiskSize      uint64
	LastUpdate    time.Time
	LastSyncBytes uint64
}
