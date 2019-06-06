package common

import "time"

// APIBackupListEntries is a list of entries for "backup list" command
type APIBackupListEntries []APIBackupListEntry

// APIBackupListEntry is an entry for a backup
type APIBackupListEntry struct {
	DiskName  string
	VMName    string
	Created   time.Time
	AuthorKey string
	Size      uint64
	AllocSize uint64
}
