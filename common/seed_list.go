package common

import "time"

// APISeedListEntries is a list of entries for "seeds" command
type APISeedListEntries []APISeedListEntry

// APISeedListEntry is an entry for a Seed
type APISeedListEntry struct {
	Name         string
	Ready        bool
	LastModified time.Time
}
