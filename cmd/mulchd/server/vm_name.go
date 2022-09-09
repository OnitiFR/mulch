package server

import (
	"fmt"
	"regexp"
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

// ParseVMName parses a VM name and returns a VMName struct
func ParseVMName(nameID string) (*VMName, error) {
	// 1: name, 3: revision (if any)
	nameRegex := regexp.MustCompile(`^([A-Za-z0-9_]+)(-r([0-9]+))?$`)
	nameMatch := nameRegex.FindStringSubmatch(nameID)

	if len(nameMatch) != 4 {
		return nil, fmt.Errorf("invalid VM name: %s", nameID)
	}

	revision := 0
	name := nameMatch[1]

	if nameMatch[3] != "" {
		var err error
		revision, err = strconv.Atoi(nameMatch[3])
		if err != nil {
			return nil, fmt.Errorf("invalid revision: %s", err)
		}
	}
	return NewVMName(name, revision), nil

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
	return app.Config.VMPrefix + name.ID()
}

func (name *VMName) String() string {
	return fmt.Sprintf("'%s' (rev %d)", name.Name, name.Revision)
}
