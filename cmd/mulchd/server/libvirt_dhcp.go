package server

import (
	"fmt"
	"strings"
	"sync"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// LibvirtDHCPLeases stores a list (map) of static DHCP leases
type LibvirtDHCPLeases struct {
	leases map[*libvirtxml.NetworkDHCPHost]bool
	mutex  sync.Mutex
}

// NewLibvirtDHCPLeases returns a new LibvirtDHCPLeases instance
func NewLibvirtDHCPLeases() *LibvirtDHCPLeases {
	return &LibvirtDHCPLeases{
		leases: make(map[*libvirtxml.NetworkDHCPHost]bool),
	}
}

// AddTransientDHCPHost will add a new transient DHCP static host
// You'll then need to remove this transient host on VM creation success/failure
func (lv *Libvirt) AddTransientDHCPHost(newHost *libvirtxml.NetworkDHCPHost, app *App) error {
	lv.dhcpLeases.mutex.Lock()
	defer lv.dhcpLeases.mutex.Unlock()

	lv.dhcpLeases.leases[newHost] = true
	return lv.rebuildDHCPStaticLeases(app)
}

// RemoveTransientDHCPHost will remove a transient DHCP lease
func (lv *Libvirt) RemoveTransientDHCPHost(newHost *libvirtxml.NetworkDHCPHost, app *App) error {
	lv.dhcpLeases.mutex.Lock()
	defer lv.dhcpLeases.mutex.Unlock()

	delete(lv.dhcpLeases.leases, newHost)
	return lv.rebuildDHCPStaticLeases(app)
}

// RebuildDHCPStaticLeases will clean static DHCP leases database
func (lv *Libvirt) RebuildDHCPStaticLeases(app *App) error {
	lv.dhcpLeases.mutex.Lock()
	defer lv.dhcpLeases.mutex.Unlock()

	return lv.rebuildDHCPStaticLeases(app)
}

// non-mutex-locked internal version of RebuildDHCPStaticLeases
func (lv *Libvirt) rebuildDHCPStaticLeases(app *App) error {
	_, err := lv.GetConnection()
	if err != nil {
		return err
	}
	previousHosts := lv.NetworkXML.IPs[0].DHCP.Hosts
	var hostsToDelete []libvirtxml.NetworkDHCPHost
	var hostsToAdd []libvirtxml.NetworkDHCPHost // mostly for old VMs (previous "format") where no static IP was set

	// search for leases to delete
	for _, host := range previousHosts {
		if !strings.HasPrefix(host.Name, app.Config.VMPrefix) {
			continue
		}
		nameID := strings.TrimPrefix(host.Name, app.Config.VMPrefix)
		vm, _ := app.VMDB.GetByNameID(nameID)
		if vm == nil {
			if lv.dhcpLeases.findByHost(host.Name) == nil {
				hostsToDelete = append(hostsToDelete, host)
			}
		}
	}

	// search for leases to add (from VM database)
	vmNames := app.VMDB.GetNames()
	for _, name := range vmNames {
		found := false
		for _, host := range previousHosts {
			if host.Name == name.LibvirtDomainName(app) {
				found = true
				break
			}
		}
		if !found {
			vm, err := app.VMDB.GetByName(name)
			if err != nil {
				return err
			}
			host := libvirtxml.NetworkDHCPHost{
				Name: name.LibvirtDomainName(app),
				MAC:  vm.AssignedMAC,
				IP:   vm.AssignedIPv4,
			}
			hostsToAdd = append(hostsToAdd, host)
		}
	}

	// search for leases to add (from transient database)
	for lease := range lv.dhcpLeases.leases {
		found := false
		for _, host := range previousHosts {
			if host.Name == lease.Name {
				found = true
				break
			}
		}
		if !found {
			hostsToAdd = append(hostsToAdd, *lease)
		}
	}

	for _, host := range hostsToDelete {
		app.Log.Tracef("remove DHCP lease for '%s/%s/%s'", host.Name, host.MAC, host.IP)
		xml, err := host.Marshal()
		if err != nil {
			return err
		}
		err = lv.Network.Update(
			libvirt.NETWORK_UPDATE_COMMAND_DELETE,
			libvirt.NETWORK_SECTION_IP_DHCP_HOST,
			-1,
			xml,
			libvirt.NETWORK_UPDATE_AFFECT_LIVE|libvirt.NETWORK_UPDATE_AFFECT_CONFIG,
		)
		if err != nil {
			return err
		}
	}

	for _, host := range hostsToAdd {
		app.Log.Tracef("add DHCP lease for '%s/%s/%s'", host.Name, host.MAC, host.IP)
		xml, err := host.Marshal()
		if err != nil {
			return err
		}
		err = lv.Network.Update(
			libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST,
			libvirt.NETWORK_SECTION_IP_DHCP_HOST,
			-1,
			xml,
			libvirt.NETWORK_UPDATE_AFFECT_LIVE|libvirt.NETWORK_UPDATE_AFFECT_CONFIG,
		)
		if err != nil {
			return err
		}
	}

	// update lv.NetworkXML
	xmldoc, err := lv.Network.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("failed GetXMLDesc: %s", err)
	}

	netcfg := &libvirtxml.Network{}
	err = netcfg.Unmarshal(xmldoc)
	if err != nil {
		return fmt.Errorf("failed Unmarshal: %s", err)
	}

	lv.NetworkXML = netcfg

	return nil
}

// findByHost returns a NetworkDHCPHost lease by its name (or nil of not found)
func (lvl *LibvirtDHCPLeases) findByHost(host string) *libvirtxml.NetworkDHCPHost {
	for lease := range lvl.leases {
		if lease.Name == host {
			return lease
		}
	}
	return nil
}
