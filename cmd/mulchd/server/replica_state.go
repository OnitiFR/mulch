package server

import "time"

// ReplicaState stores metadata about a replica received from a peer (receiver side).
//
// One entry exists for each .raw replica file managed by the ReplicationReceiver:
// the database is maintained automatically through the prepare/sync/cleanup
// endpoints (see ReplicationReceiver). It carries just enough information to list
// replicas, identify their origin, and — later — promote them to local VMs (the
// raw VM TOML config is kept up to date for that purpose).
type ReplicaState struct {
	Name          string
	Revision      int
	Origin        string // API key comment of the source peer that sent this replica
	DiskSize      uint64
	Config        string // raw VM TOML config (for future promote)
	LastUpdate    time.Time
	LastSyncBytes uint64

	// ConsistentSnapshot is true whenever the .raw holds a complete FSFreeze
	// point (the normal state after any successful sync). It is cleared only
	// while a full copy rewrites the whole image in place (Prepare), and set
	// again once that full copy — or any later incremental — completes. A
	// promote must refuse a replica whose ConsistentSnapshot is false.
	ConsistentSnapshot bool

	// Applying mirrors the presence of a committed staging journal
	// (<vm>.mrpl.journal): true between the atomic commit of an incremental
	// sync and the completion of its replay onto the .raw. It is informational
	// (e.g. for "replica list"); the authoritative recovery signal is the
	// journal file itself (see recoverJournals).
	Applying bool

	// Promoting is true while a 'replica promote' is running: incoming
	// prepare/sync calls are durably refused (even across a mulchd restart)
	// until the promote either completes (Promoted) or is rolled back.
	Promoting bool

	// Diverged is set when a failed promote booted the guest: the VM wrote to
	// the .raw during the failed boot, so the image no longer matches the
	// source's last checkpoint. It stays promotable (crash-consistent image of
	// the failed boot), but incremental syncs are refused (deltas would apply
	// on a diverged base): the source is told to redo a full copy, which
	// clears the flag.
	Diverged bool

	// Promoted marks a tombstone: the replica was promoted to a local VM and
	// its .raw was moved to the disks pool. The entry is kept so the original
	// source peer gets a "stand-down" refusal instead of silently recreating
	// a replica of a VM now running here. Only an explicit 'replica delete'
	// clears it.
	Promoted bool
}

// ID returns the unique VM ID (name + revision) for this replica
func (s *ReplicaState) ID() string {
	return NewVMName(s.Name, s.Revision).ID()
}
