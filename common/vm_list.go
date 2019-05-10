package common

// APIVMListEntries is a list of entries for "vm list" command
type APIVMListEntries []APIVMListEntry

// APIVMListEntry is an entry for a VM
type APIVMListEntry struct {
	Name      string
	Revision  int
	Active    bool
	LastIP    string `json:"last_ip"`
	State     string
	Locked    bool
	WIP       string
	SuperUser string
	AppUser   string
}
