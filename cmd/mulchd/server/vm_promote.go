package server

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// NeutralizeTOMLReplicationPeer returns the TOML content with any top-level
// replication_peer assignment commented out (with an explanatory marker
// carrying the date and the author, i.e. the API key comment).
//
// A promoted VM must not replicate anywhere (D7, see HA_PROMOTE.md): the
// stored peer name belongs to the source host — usually unknown here (parse
// error), or worse, known (symmetric peering) and the VM would silently
// replicate back to its origin. The neutralization must be durable, in the
// TOML itself: FileContent is the source of truth re-parsed by every future
// rebuild or redefine.
func NeutralizeTOMLReplicationPeer(content string, author string) string {
	marker := "  # neutralized by 'replica promote' on " + time.Now().Format("2006-01-02 15:04:05")
	if author != "" {
		marker += " by " + author
	}

	lines := strings.Split(content, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		rest, found := strings.CutPrefix(trimmed, "replication_peer")
		if !found || !strings.HasPrefix(strings.TrimSpace(rest), "=") {
			continue
		}
		lines[i] = "# " + line + marker
		changed = true
	}

	if !changed {
		return content
	}
	return strings.Join(lines, "\n")
}

// PromoteReplica turns a replica held by this peer into a real local VM (the
// failover path, see HA_PROMOTE.md): it durably refuses any further incoming
// replication for this VM name, moves the .raw into the disks pool (a rename,
// no copy), boots a VM from it as-is (NewVMFromExistingDisk) and finally turns
// the replica entry into a "promoted" tombstone. On failure the disk is moved
// back and the replica accepts syncs again.
//
// Everything is local (plus the parent proxy for domains): the source peer is
// assumed unreachable and is never contacted.
func PromoteReplica(vmID string, authorKey string, app *App, log *Log) (*VMName, error) {
	state, err := app.ReplicationReceiver.BeginPromote(vmID)
	if err != nil {
		return nil, err
	}

	vmCreated := false
	booted := false
	defer func() {
		if !vmCreated {
			app.ReplicationReceiver.AbortPromote(vmID, booted)
		}
	}()

	configTOML := NeutralizeTOMLReplicationPeer(state.Config, authorKey)
	conf, err := NewVMConfigFromTomlReader(strings.NewReader(configTOML), app)
	if err != nil {
		return nil, fmt.Errorf("decoding replica config: %s", err)
	}
	if conf.ReplicationPeer != "" {
		return nil, fmt.Errorf("replication_peer neutralization failed (still set to '%s')", conf.ReplicationPeer)
	}
	if conf.Name != state.Name {
		return nil, fmt.Errorf("replica config name mismatch: config says '%s', replica is '%s'", conf.Name, state.Name)
	}

	// move (rename, same filesystem: instantaneous) the .raw into the libvirt
	// disks pool — from here on, the only sane copy of the data has moved, so
	// any failure must move it back
	srcPath := app.ReplicationReceiver.RawFilePath(vmID)
	diskName := vmID + ".raw"
	dstPath := path.Clean(app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName)

	if _, err := os.Stat(dstPath); err == nil {
		return nil, fmt.Errorf("disk '%s' already exists in the disks pool", diskName)
	}

	log.Infof("moving replica disk into the disks pool (%s)", diskName)
	if err := os.Rename(srcPath, dstPath); err != nil {
		return nil, fmt.Errorf("can't move replica disk: %s", err)
	}

	rollbackMove := func() {
		if errR := os.Rename(dstPath, srcPath); errR != nil {
			log.Errorf("CRITICAL: can't move replica disk back to '%s', manual action needed: %s", srcPath, errR)
			return
		}
		app.Libvirt.Pools.Disks.Refresh(0)
	}

	// make libvirt see the new volume (raw format is auto-detected)
	if err := app.Libvirt.Pools.Disks.Refresh(0); err != nil {
		rollbackMove()
		return nil, fmt.Errorf("can't refresh disks pool: %s", err)
	}

	_, vmName, err := NewVMFromExistingDisk(conf, diskName, true, authorKey, &booted, app, log)
	if err != nil {
		log.Warningf("promote failed, moving replica disk back")
		rollbackMove()
		return nil, err
	}

	// the VM is up: never roll it back past this point
	vmCreated = true

	if err := app.ReplicationReceiver.FinishPromote(vmID); err != nil {
		log.Errorf("VM '%s' is promoted but the replica tombstone can't be finalized: %s", vmName, err)
	}

	return vmName, nil
}

// NewVMFromExistingDisk creates a real local VM from a disk image already
// present in the "disks" pool (named diskName, raw format), WITHOUT cloning a
// seed and WITHOUT running prepare/install/restore scripts: the disk already
// carries all state. This is the core of "replica promote" (see PromoteReplica).
//
// It assigns a fresh MAC/IP and a NEW SecretUUID so that, on first boot,
// cloud-init sees a different instance-id and re-runs as a new instance
// (re-injecting this peer's SSH super user key) and the phone-home then matches
// in the local VMDB. The domain is defined with disk driver type "raw".
//
// booted is set to true as soon as the guest starts: from that point the disk
// content is mutated, even if the VM creation fails afterwards (the caller
// uses it to flag the replica as diverged on rollback).
func NewVMFromExistingDisk(vmConfig *VMConfig, diskName string, active bool, authorKey string, booted *bool, app *App, log *Log) (*VM, *VMName, error) {
	log.Infof("promoting VM '%s' from existing disk '%s'", vmConfig.Name, diskName)

	commit := false

	secretUUID, err := uuid.NewV4()
	if err != nil {
		return nil, nil, err
	}

	vm := &VM{
		App:                  app,
		SecretUUID:           secretUUID.String(),
		Config:               vmConfig,
		AuthorKey:            authorKey,
		MulchSuperUserSSHKey: app.Config.MulchSuperUserSSHKey,
		InitDate:             time.Now(),
		Locked:               false,
		WIP:                  VMOperationNone,
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, nil, err
	}

	if !IsValidName(vmConfig.Name) {
		return nil, nil, fmt.Errorf("name '%s' is invalid", vmConfig.Name)
	}

	revision := app.VMDB.GetNextRevisionForName(vmConfig.Name)
	vmName := NewVMName(vmConfig.Name, revision)
	domainName := vmName.LibvirtDomainName(app)

	_, err = conn.LookupDomainByName(domainName)
	if err == nil {
		return nil, nil, fmt.Errorf("VM '%s' already exists in libvirt", domainName)
	}

	if active {
		// same early checks as NewVM (VMDB.Add re-does them at the end; failing
		// now avoids a full boot for nothing).
		// TODO (HA_PROMOTE.md step 3, "force" + pin): CheckDomainsConflicts
		// includes the parent proxy check, which refuses the promote as long as
		// the (dead) source peer still owns the domains there. The promote path
		// will have to skip it.
		err = CheckDomainsConflicts(app.VMDB, vmConfig.Domains, vmName.Name, app.Config)
		if err != nil {
			return nil, nil, err
		}
		err = CheckPortsConflicts(app.VMDB, vmConfig.Ports, vmName.Name, log)
		if err != nil {
			return nil, nil, err
		}
	}

	// missing secrets would only surface when the guest fetches its cloud-init
	// data, as a BuildTimeout later: fail now instead (D4)
	_, err = vm.GetSecretsMap()
	if err != nil {
		return nil, nil, err
	}

	vm.AssignedMAC = RandomUniqueMAC(app)
	vm.AssignedIPv4, err = RandomUniqueIPv4(app)
	if err != nil {
		return nil, nil, err
	}

	app.VMDB.AddToGreenhouse(vm, vmName)
	defer app.VMDB.DeleteFromGreenhouse(vmName)

	// static DHCP lease so the guest gets exactly AssignedIPv4 (matches the
	// clean-traffic nwfilter), like NewVM does.
	transientLease := &libvirtxml.NetworkDHCPHost{
		Name: domainName,
		MAC:  vm.AssignedMAC,
		IP:   vm.AssignedIPv4,
	}
	app.Libvirt.AddTransientDHCPHost(transientLease, app)
	defer app.Libvirt.RemoveTransientDHCPHost(transientLease, app)

	// define domain (no seed clone, no resize: the disk is already in place)
	log.Infof("defining vm domain (%s)", domainName)
	xml, err := os.ReadFile(app.Config.GetTemplateFilepath("vm.xml"))
	if err != nil {
		return nil, nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(string(xml))
	if err != nil {
		return nil, nil, err
	}

	domcfg.Name = domainName
	domcfg.Memory.Unit = "bytes"
	domcfg.Memory.Value = uint(vm.Config.RAMSize)
	domcfg.CurrentMemory.Unit = "bytes"
	domcfg.CurrentMemory.Value = uint(vm.Config.RAMSize)
	domcfg.VCPU.Value = uint(vm.Config.CPUCount)

	serial := "ds=nocloud-net;s=http://" + app.Libvirt.NetworkXML.IPs[0].Address + ":" + strconv.Itoa(app.Config.InternalServerPort) + "/cloud-init/" + vm.SecretUUID + "/"
	serialFound := false
	for s, sysinfo := range domcfg.SysInfo {
		for i, entry := range sysinfo.SMBIOS.System.Entry {
			if entry.Name == "version" {
				domcfg.SysInfo[s].SMBIOS.System.Entry[i].Value = Version
			}
			if entry.Name == "serial" {
				serialFound = true
				domcfg.SysInfo[s].SMBIOS.System.Entry[i].Value = serial
			}
		}
	}
	if !serialFound {
		return nil, nil, errors.New("vm xml file: smbios serial entry not found")
	}

	foundDisks := 0
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			disk.Driver.Type = "raw" // promote: boot the replicated raw as-is (no conversion)
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
			foundDisks++
		}
	}
	if foundDisks != 1 {
		return nil, nil, errors.New("vm xml file: a single disk with 'ua-mulch-disk' alias is required")
	}

	foundInterfaces := 0
	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias != nil && intf.Alias.Name == VMNetworkAliasBridge {
			intf.Source.Bridge.Bridge = app.Libvirt.NetworkXML.Bridge.Name
			intf.MAC.Address = vm.AssignedMAC
			if intf.FilterRef == nil {
				return nil, nil, errors.New("vm xml file: no filterref found for network interface")
			}
			if intf.FilterRef.Filter != AppNWFilter {
				return nil, nil, fmt.Errorf("vm xml file: need filterref '%s'", AppNWFilter)
			}
			foundParamIP := 0
			foundParamGateway := 0
			for index, param := range intf.FilterRef.Parameters {
				if param.Name == "IP" {
					intf.FilterRef.Parameters[index].Value = vm.AssignedIPv4
					foundParamIP++
				}
				if param.Name == "GATEWAY_MAC" {
					intf.FilterRef.Parameters[index].Value = app.Libvirt.NetworkXML.MAC.Address
					foundParamGateway++
				}
			}
			if foundParamIP != 1 || foundParamGateway != 1 {
				return nil, nil, errors.New("vm xml file: need exactly one IP and one GATEWAY_MAC filter param")
			}
			foundInterfaces++
		}
	}
	if foundInterfaces != 1 {
		return nil, nil, errors.New("vm xml file: need exactly one interface with 'ua-mulch-bridge' alias")
	}

	xml2, err := domcfg.Marshal()
	if err != nil {
		return nil, nil, err
	}

	dom, err := conn.DomainDefineXML(string(xml2))
	if err != nil {
		return nil, nil, err
	}
	defer dom.Free()

	defer func() {
		if !commit {
			log.Infof("rollback, deleting vm %s", vmName)
			dom.Destroy() // stop (if needed)
			if errDef := dom.Undefine(); errDef != nil {
				log.Errorf("can't delete vm: %s", errDef)
			}
		}
	}()

	libvirtUUID, err := dom.GetUUIDString()
	if err != nil {
		return nil, nil, err
	}
	vm.LibvirtUUID = libvirtUUID

	log.Infof("vm: first boot (cloud-init re-run expected, new SecretUUID)")
	err = dom.Create()
	if err != nil {
		return nil, nil, err
	}
	*booted = true

	phone := app.PhoneHome.Register(secretUUID.String())
	defer phone.Unregister()

	log.Tracef("waiting for phone call (timeout: %s)", vm.Config.BuildTimeout)
	timeout := time.After(vm.Config.BuildTimeout)
	for done := false; !done; {
		select {
		case <-timeout:
			return nil, nil, errors.New("vm init is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			if call.CloutInit {
				done = true
				log.Info("vm phoned home, cloud-init was successful")
				vm.LastIP = call.RemoteIP
			}
		case <-time.After(5 * time.Second):
			state, _, errG := dom.GetState()
			if errG != nil {
				return nil, nil, errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return nil, nil, errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				return nil, nil, errors.New("vm unexpectedly stopped")
			}
		}
	}
	phone.Unregister()

	log.Infof("saving VM in database")
	err = app.VMDB.Add(vm, vmName, active)
	if err != nil {
		return nil, nil, err
	}

	commit = true
	return vm, vmName, nil
}
