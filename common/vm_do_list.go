package common

// APIVMDoListEntries is a list of actions for "do" command
type APIVMDoListEntries []APIVMDoListEntry

// APIVMDoListEntry is an entry for an action
type APIVMDoListEntry struct {
	Name        string
	User        string
	Description string
}
