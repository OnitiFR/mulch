package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// IPStringToInt convert an IPv4 string to a unsigned int 32
func IPStringToInt(ip string) uint32 {
	var long uint32
	ip4 := net.ParseIP(ip).To4()
	binary.Read(bytes.NewBuffer(ip4), binary.BigEndian, &long)
	return long
}

// IPIntToString convert an uint32 IPv4 to a string
func IPIntToString(ipn uint32) string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipn)
	return ip.String()
}

// RandomUniqueMAC generate a random unique (among other Mulch VMs) MAC address
// (we use QEMU MAC prefix)
func RandomUniqueMAC(app *App) string {
	vmNames := app.VMDB.GetNames()
	mac := ""
	for {
		unique := true
		mac = fmt.Sprintf("52:54:00:%02x:%02x:%02x", app.Rand.Intn(255), app.Rand.Intn(255), app.Rand.Intn(255))
		for _, name := range vmNames {
			vm, err := app.VMDB.GetByName(name)
			if err == nil && vm.AssignedMAC == mac {
				unique = false
			}
		}
		if unique {
			break
		}
		app.Log.Tracef("(rare) MAC conflict for %s, generating a new one", mac)
	}
	return mac
}

// RandomUniqueIPv4 generate a random unique IPv4 (among other Mulch VMs)
// inside libvirt DHCP range, excluding other "external" static leases
func RandomUniqueIPv4(app *App) (string, error) {
	vmNames := app.VMDB.GetNames()
	ip := ""

	ipStart := IPStringToInt(app.Libvirt.NetworkXML.IPs[0].DHCP.Ranges[0].Start)
	ipEnd := IPStringToInt(app.Libvirt.NetworkXML.IPs[0].DHCP.Ranges[0].End)

	if ipStart == 0 || ipEnd == 0 || ipStart >= ipEnd {
		return "", errors.New("invalid network DHCP range")
	}

	diff := ipEnd - ipStart

	for {
		unique := true
		rnd := app.Rand.Int63n(int64(diff) + 1)
		ip = IPIntToString(ipStart + uint32(rnd))
		// other VMs
		for _, name := range vmNames {
			vm, err := app.VMDB.GetByName(name)
			if err == nil && vm.AssignedIPv4 == ip {
				unique = false
			}
		}
		// static leases
		for _, host := range app.Libvirt.NetworkXML.IPs[0].DHCP.Hosts {
			if host.IP == ip {
				unique = false
			}
		}
		if unique {
			break
		}
		app.Log.Tracef("(rare) IPv4 conflict for %s, generating a new one", ip)
	}

	return ip, nil
}
