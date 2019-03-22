package common

import "time"

// APIVmInfos expose a few VM informations
type APIVmInfos struct {
	Name      string
	Seed      string
	CPUCount  int
	RAMSize   uint64
	Hostname  string
	SuperUser string
	AppUser   string
	InitDate  time.Time
	AuthorKey string
	Locked    bool
	Up        bool
}
