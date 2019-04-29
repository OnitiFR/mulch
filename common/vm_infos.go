package common

import "time"

// APIVMInfos expose a few VM informations
type APIVMInfos struct {
	Name                string
	Seed                string
	CPUCount            int
	RAMSizeMB           uint64
	DiskSizeMB          uint64
	AllocatedDiskSizeMB uint64
	BackupDiskSizeMB    uint64
	Hostname            string
	SuperUser           string
	AppUser             string
	InitDate            time.Time
	AuthorKey           string
	Locked              bool
	Up                  bool
}
