package server

import (
	"fmt"
	"strconv"
)

// VMName hosts what makes a VM unique: a name and a revision
type VMName struct {
	Name     string
	Revision int
}

// NewVMName instanciates a new VMName struct
func NewVMName(name string, revision int) *VMName {
	return &VMName{
		Name:     name,
		Revision: revision,
	}
}

// ID returns a unique ID for the VM
func (name *VMName) ID() string {
	if name.Revision == 0 {
		return name.Name
	}
	return name.Name + "-r" + strconv.Itoa(name.Revision)
}

// LibvirtDomainName returns the libvirt domain name (using app prefix)
func (name *VMName) LibvirtDomainName(app *App) string {
	return app.Config.VMPrefixTODOXF + name.ID()
}

func (name *VMName) String() string {
	return fmt.Sprintf("'%s' (rev %d)", name.Name, name.Revision)
}
