package common

// APIVmListEntries is a list of entries for "vm list" command
type APIVmListEntries []APIVmListEntry

// APIVmListEntry is an entry for a VM
type APIVmListEntry struct {
	Name      string
	LastIP    string `json:"last_ip"`
	State     string
	Locked    bool
	WIP       string
	SuperUser string
	AppUser   string
}
