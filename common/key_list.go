package common

// APIKeyListEntries is a list of entries for "backup list" command
type APIKeyListEntries []APIKeyListEntry

// APIKeyListEntry is an entry for a backup
type APIKeyListEntry struct {
	Comment string
}
