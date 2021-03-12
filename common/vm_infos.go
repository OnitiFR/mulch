package common

import "time"

// APIVMInfos expose a few VM informations
type APIVMInfos struct {
	Name                string
	Revision            int
	Active              bool
	Up                  bool
	Seed                string
	CPUCount            int
	RAMSizeMB           uint64
	DiskSizeMB          uint64
	AllocatedDiskSizeMB uint64
	BackupDiskSizeMB    uint64
	Hostname            string
	Domains             []string
	SuperUser           string
	AppUser             string
	InitDate            time.Time
	LastRebuildDuration time.Duration
	LastRebuildDowntime time.Duration
	AuthorKey           string
	Locked              bool
	AssignedIPv4        string
	AssignedMAC         string
	DoActions           []string
	Tags                []string
}
