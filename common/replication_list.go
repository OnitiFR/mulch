package common

import "time"

// APIReplicationEntries is a list of entries for "replication list"
type APIReplicationEntries []APIReplicationEntry

// APIReplicationEntry is an entry for a replicated VM (source side)
type APIReplicationEntry struct {
	Name               string
	Revision           int
	Active             bool
	PeerName           string
	Status             string
	FullCopyDone       bool
	LastSyncTime       time.Time
	LastSyncDuration   time.Duration
	LastSyncBytes      uint64
	NextSyncTime       time.Time // next scheduled sync (zero = none, e.g. sync in progress)
	LastError          string
	LastErrorTime      time.Time
	ConsecutiveErrors  int
	BackoffInterval    time.Duration // effective interval (0 = no backoff or unknown)
	ConfiguredInterval time.Duration // configured replication_interval
	Alerted            bool          // true once an alert was sent for the current incident
	AlertDelay         time.Duration // staleness threshold (no successful sync) that triggers an alert
}
