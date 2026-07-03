package common

import "time"

// APIReplicaEntries is a list of entries for "replica list"
type APIReplicaEntries []APIReplicaEntry

// APIReplicaEntry is an entry for a replica held by this peer (receiver side)
type APIReplicaEntry struct {
	Name          string
	Revision      int
	Origin        string
	Status        string // "idle", "syncing", "promoting" or "promoted" (tombstone)
	DiskSize      uint64
	LastUpdate    time.Time
	LastSyncBytes uint64

	// ConsistentSnapshot is false while an initial/forced full copy has not
	// completed: the .raw is partial and not safe to promote. An incremental
	// sync preserves consistency, so it stays true once a full copy succeeded.
	ConsistentSnapshot bool
}
