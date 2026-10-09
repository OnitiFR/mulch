package server

import (
	"errors"
	"fmt"

	"libvirt.org/go/libvirtxml"
)

// The vm.xml and disk.xml templates are user-editable, and libvirtxml
// unmarshals "choice" elements (<source>, <sysinfo>, …) as a union of
// pointers, selected by the 'type' attribute of the parent element: an
// unexpected 'type' leaves the pointer nil *without* any unmarshal error.
// Many top level elements (<memory>, <vcpu>, <devices>, …) are pointers
// too, and are nil when absent.
//
// Since we dereference all this to build the domain, the checks below turn
// what would otherwise be a nil dereference into a plain error message.
// This matters: NewVM runs in its own goroutine (stream route, seeder),
// so a panic there takes the whole mulchd down.

// vmTemplateCheck ensures a vm.xml template provides every element we
// need to dereference when defining a domain
func vmTemplateCheck(domcfg *libvirtxml.Domain) error {
	if domcfg.Memory == nil {
		return errors.New("vm xml file: <memory> element is required")
	}
	if domcfg.CurrentMemory == nil {
		return errors.New("vm xml file: <currentMemory> element is required")
	}
	if domcfg.VCPU == nil {
		return errors.New("vm xml file: <vcpu> element is required")
	}
	if domcfg.Devices == nil {
		return errors.New("vm xml file: <devices> section is required")
	}

	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias == nil || disk.Alias.Name != VMStorageAliasDisk {
			continue
		}
		if disk.Source == nil || disk.Source.File == nil {
			return fmt.Errorf("vm xml file: disk with '%s' alias must be a type='file' disk, with a <source file='...'/> element", VMStorageAliasDisk)
		}
	}

	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias == nil || intf.Alias.Name != VMNetworkAliasBridge {
			continue
		}
		if intf.Source == nil || intf.Source.Bridge == nil {
			return fmt.Errorf("vm xml file: interface with '%s' alias must be a type='bridge' interface, with a <source bridge='...'/> element", VMNetworkAliasBridge)
		}
		if intf.MAC == nil {
			return fmt.Errorf("vm xml file: interface with '%s' alias requires a <mac address='...'/> element", VMNetworkAliasBridge)
		}
	}

	return nil
}

// vmBackupDiskTemplateCheck is the disk.xml counterpart of vmTemplateCheck,
// for the hotplugged backup disk
func vmBackupDiskTemplateCheck(diskcfg *libvirtxml.DomainDisk) error {
	if diskcfg.Alias == nil {
		return errors.New("disk xml file: <alias> element is required")
	}
	if diskcfg.Source == nil || diskcfg.Source.File == nil {
		return errors.New("disk xml file: a type='file' disk with a <source file='...'/> element is required")
	}
	if diskcfg.Target == nil {
		return errors.New("disk xml file: <target> element is required")
	}

	return nil
}
