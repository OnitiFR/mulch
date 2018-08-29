package server

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	"github.com/satori/go.uuid"
)

// Aliases for vm.xml file
const (
	VMStorageAliasDisk      = "ua-mulch-disk"
	VMStorageAliasCloudInit = "ua-mulch-cloudinit"
	VMNetworkAliasBridge    = "ua-mulch-bridge"
)

// VM defines a virtual machine ("domain")
type VM struct {
	LibvirtUUID string
	SecretUUID  string
	App         *App
	Config      *VMConfig
	LastIP      string
	Locked      bool
}

// NewVM builds a new virtual machine from config
// TODO: this function is HUUUGE and needs to be splitted. It's tricky
// because there's a "transaction" here.
func NewVM(vmConfig *VMConfig, app *App, log *Log) (*VM, error) {
	log.Infof("creating new VM '%s'", vmConfig.Name)

	commit := false

	secretUUID, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}

	vm := &VM{
		App:        app,
		SecretUUID: secretUUID.String(),
		Config:     vmConfig, // copy()? (deep)
		Locked:     false,
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	if !IsValidTokenName(vmConfig.Name) {
		return nil, fmt.Errorf("name '%s' is invalid (need only letters, numbers and underscore, do not start with a number)", vmConfig.Name)
	}

	_, err = app.VMDB.GetByName(vmConfig.Name)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists in database", vmConfig.Name)
	}

	domainName := app.Config.VMPrefix + vmConfig.Name

	_, err = conn.LookupDomainByName(domainName)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists in libvirt", domainName)
	}
	errDetails := err.(libvirt.Error)
	if errDetails.Domain != libvirt.FROM_QEMU || errDetails.Code != libvirt.ERR_NO_DOMAIN {
		return nil, fmt.Errorf("Unexpected error: %s", err)
	}

	ciName := "ci-" + vmConfig.Name + ".img"
	diskName := vmConfig.Name + ".qcow2"

	// 1 - copy from reference image
	log.Infof("creating VM disk '%s'", diskName)
	err = app.Libvirt.CreateDiskFromSeed(
		vmConfig.SeedImage,
		diskName,
		app.Config.configPath+"/templates/volume.xml",
		log)

	if err != nil {
		return nil, err
	}

	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			log.Infof("rollback, deleting disk '%s'", diskName)
			vol, errDef := app.Libvirt.Pools.Disks.LookupStorageVolByName(diskName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, diskName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", errDef, diskName)
				return
			}
		}
	}()

	// 2 - resize disk
	err = app.Libvirt.ResizeDisk(diskName, vmConfig.DiskSize, log)
	if err != nil {
		return nil, err
	}

	// 3 - Cloud-Init files
	log.Infof("creating Cloud-Init image for '%s'", vmConfig.Name)
	err = CloudInitCreate(ciName, vm, app, log)
	if err != nil {
		return nil, err
	}
	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			log.Infof("rollback, deleting cloud-init image '%s'", ciName)
			vol, errDef := app.Libvirt.Pools.CloudInit.LookupStorageVolByName(ciName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, ciName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", errDef, ciName)
				return
			}
		}
	}()

	// 4 - define domain
	log.Infof("defining vm domain (%s)", domainName)
	xml, err := ioutil.ReadFile(app.Config.configPath + "/templates/vm.xml")
	if err != nil {
		return nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(string(xml))
	if err != nil {
		return nil, err
	}

	domcfg.Name = domainName

	domcfg.Memory.Unit = "bytes"
	domcfg.Memory.Value = uint(vm.Config.RAMSize)
	domcfg.CurrentMemory.Unit = "bytes"
	domcfg.CurrentMemory.Value = uint(vm.Config.RAMSize)

	domcfg.VCPU.Value = vm.Config.CPUCount

	foundDisks := 0
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
			foundDisks++
		}
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			disk.Source.File.File = app.Libvirt.Pools.CloudInitXML.Target.Path + "/" + ciName
			foundDisks++
		}
	}

	if foundDisks != 2 {
		return nil, errors.New("vm xml file: disks with 'ua-mulch-disk' and 'ua-mulch-cloudinit' aliases are required, see sample file")
	}

	foundInterfaces := 0
	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias != nil && intf.Alias.Name == VMNetworkAliasBridge {
			intf.Source.Bridge.Bridge = app.Libvirt.NetworkXML.Bridge.Name
			intf.MAC.Address = fmt.Sprintf("52:54:00:%02x:%02x:%02x", app.Rand.Intn(255), app.Rand.Intn(255), app.Rand.Intn(255))
			foundInterfaces++
		}
	}

	if foundInterfaces != 1 {
		return nil, fmt.Errorf("vm xml file: found %d interface(s) with 'ua-mulch-bridge' alias, one is needed", foundInterfaces)
	}

	xml2, err := domcfg.Marshal()
	if err != nil {
		return nil, err
	}

	dom, err := conn.DomainDefineXML(string(xml2))
	if err != nil {
		return nil, err
	}
	defer dom.Free() // remember: "deferred calls are executed in last-in-first-out order"

	defer func() {
		if !commit {
			log.Infof("rollback, deleting vm '%s'", vm.Config.Name)
			errDef := dom.Undefine()
			if errDef != nil {
				return
			}
		}
	}()

	libvirtUUID, err := dom.GetUUIDString()
	if err != nil {
		return nil, err
	}
	vm.LibvirtUUID = libvirtUUID

	log.Infof("vm: first boot (cloud-init)")
	if vmConfig.InitUpgrade {
		log.Info("cloud-init will upgrade package, it may take a while…")
	}
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	phone := app.PhoneHome.Register(secretUUID.String())
	defer phone.Unregister()

	phoned := false
	for done := false; done == false; {
		select {
		case <-time.After(10 * time.Minute):
			return nil, errors.New("vm init is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			phoned = true
			log.Info("vm phoned home, cloud-init was successful")
			vm.LastIP = call.RemoteIP
		case <-time.After(1 * time.Second):
			log.Trace("checking vm state")
			state, _, errG := dom.GetState()
			if errG != nil {
				return nil, errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return nil, errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				log.Info("vm is now down")
				done = true
			}
		}
	}

	if phoned == false {
		return nil, errors.New("vm is down but didn't phoned home, something went wrong during cloud-init")
	}

	// if all is OK, remove and delete cloud-init image
	// EDIT: no! Cloud-init service is screwed on next boot (at least on debian)
	// log.Infof("removing cloud-init filesystem and volume")
	// dom2, err := vmDeleteCloudInitDisk(dom, app.Libvirt.Pools.CloudInit, conn)
	// if err != nil {
	// 	return nil, err
	// }
	// defer dom2.Free()

	// start the VM again
	log.Infof("starting vm")
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	// wait the vm's phone call
	for done := false; done == false; {
		select {
		case <-time.After(5 * time.Minute):
			dom.Destroy()
			return nil, errors.New("vm start is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			done = true
			log.Info("vm phoned home, boot successful")
			if call.RemoteIP != vm.LastIP {
				log.Warningf("vm IP changed since cloud-init call (from '%s' to '%s')", vm.LastIP, call.RemoteIP)
				vm.LastIP = call.RemoteIP
			}
		}
	}

	// TODO: run prepare scripts

	// all is OK, commit (= no defer) and save vm to DB
	log.Infof("saving VM in database")
	err = app.VMDB.Add(vm)
	if err != nil {
		return nil, err
	}
	commit = true
	return vm, nil
}

func vmDeleteCloudInitDisk(dom *libvirt.Domain, pool *libvirt.StoragePool, conn *libvirt.Connect) (*libvirt.Domain, error) {
	// 1 - remove filesystem from domain
	xmldoc, err := dom.GetXMLDesc(0)
	if err != nil {
		return nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return nil, err
	}

	ciName := ""
	tmp := domcfg.Devices.Disks[:0]
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			ciName = path.Base(disk.Source.File.File)
		} else {
			tmp = append(tmp, disk)
		}
	}
	if ciName == "" {
		return nil, fmt.Errorf("clound-init clean: disk with '%s' alias not found", VMStorageAliasCloudInit)
	}

	domcfg.Devices.Disks = tmp

	out, err := domcfg.Marshal()
	if err != nil {
		return nil, err
	}

	// update the domain
	dom2, err := conn.DomainDefineXML(string(out))
	if err != nil {
		return nil, err
	}

	// 2 - delete volume
	vol, err := pool.LookupStorageVolByName(ciName)
	if err != nil {
		return nil, err
	}
	defer vol.Free()
	err = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if err != nil {
		return nil, err
	}

	vol.Free()
	return dom2, nil
}

// VMStopByName stops a VM using its (libvirt) name
// and waits until the VM is shutoff. (or timeouts)
func VMStopByName(name string, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", name)
	}
	defer domain.Free()

	// get current state
	state, _, errG := domain.GetState()
	if errG != nil {
		return errG
	}
	if state != libvirt.DOMAIN_RUNNING {
		return errors.New("VM is not up")
	}

	// shutdown
	errS := domain.Shutdown()
	if errS != nil {
		return errS
	}

	// wait shutoff state
	for done := false; done == false; {
		select {
		case <-time.After(5 * time.Minute):
			return errors.New("vm shutdown is too long")
		case <-time.After(1 * time.Second):
			log.Trace("checking vm state")
			state, _, errG := domain.GetState()
			if errG != nil {
				return errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				done = true
			}
		}
	}

	return nil
}

// VMStartByName starts a VM using its (libvirt) name
// and waits until the VM phones home. (or timeouts)
func VMStartByName(name string, secretUUID string, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", name)
	}
	defer domain.Free()

	// get current state
	state, _, errG := domain.GetState()
	if errG != nil {
		return errG
	}
	if state != libvirt.DOMAIN_SHUTOFF {
		return errors.New("VM is not down")
	}

	err = domain.Create()
	if err != nil {
		return err
	}

	log.Info("started, waiting phone call")

	phone := app.PhoneHome.Register(secretUUID)
	defer phone.Unregister()

	for done := false; done == false; {
		select {
		case <-time.After(10 * time.Minute):
			return errors.New("vm is too long to start, something probably went wrong")
		case <-phone.PhoneCalls:
			done = true
			log.Info("vm phoned home")
		}
	}

	return nil
}

// VMLockUnlock will lock or unlock a VM, preventing it from deletion
func VMLockUnlock(vmName string, locked bool, vmdb *VMDatabase) error {
	vm, err := vmdb.GetByName(vmName)
	if err != nil {
		return err
	}

	vm.Locked = locked
	vmdb.Update()
	return nil
}
