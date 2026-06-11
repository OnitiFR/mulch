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
}

// ID returns the unique VM ID (name + revision) for this replica
func (s *ReplicaState) ID() string {
	return NewVMName(s.Name, s.Revision).ID()
}
