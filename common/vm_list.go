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

// APIVMBasicListEntries is a light variant of APIVMListEntries
// This is useful for quick requests (like completion)
type APIVMBasicListEntries []APIVMBasicListEntry

// APIVMBasicListEntry is a basic (light) entry for a VM
type APIVMBasicListEntry struct {
	Name string
}
