package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	"github.com/satori/go.uuid"
)

// VM defines a virtual machine ("domain")
type VM struct {
	LibvirtUUID string
	SecretUUID  string
	App         *App
	Config      *VMConfig
}

// VMConfig stores needed parameters for a new VM
type VMConfig struct {
	Name      string
	Hostname  string
	SeedImage string
	DiskSize  uint64
	RAMSize   uint64
	CPUCount  int
	// + prepare scripts
	// + save scripts
	// + restore scripts
}

// NewVM builds a new virtual machine from config
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
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	domainName := app.Config.VMPrefix + vmConfig.Name

	_, err = conn.LookupDomainByName(domainName)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists", vmConfig.Name)
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
				log.Errorf("failed LookupStorageVolByName: %s (%s)", err, diskName)
				return
			}
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", err, diskName)
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
	err = CloudInitCreate(ciName,
		vm.SecretUUID,
		vm.Config.Hostname,
		app.Config.configPath+"/templates/volume.xml",
		app.Config.configPath+"/templates/ci-user-data.yml",
		app,
		log)
	if err != nil {
		return nil, err
	}
	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			log.Infof("rollback, deleting cloud-init image '%s'", ciName)
			vol, errDef := app.Libvirt.Pools.CloudInit.LookupStorageVolByName(ciName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", err, ciName)
				return
			}
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", err, ciName)
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
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-disk" {
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
			foundDisks++
		}
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-cloudinit" {
			disk.Source.File.File = app.Libvirt.Pools.CloudInitXML.Target.Path + "/" + ciName
			foundDisks++
		}
	}

	if foundDisks != 2 {
		return nil, errors.New("vm xml file: disks with 'ua-mulch-disk' and 'ua-mulch-cloudinit' aliases are required, see sample file")
	}

	foundInterfaces := 0
	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias != nil && intf.Alias.Name == "ua-mulch-bridge" {
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

	log.Infof("vm: first boot (cloud-init will upgrade package, please wait…)")
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	for done := false; done == false; {
		select {
		case <-time.After(15 * time.Minute):
			return nil, errors.New("vm creation is too long, something probably went wrong")
			// case for phoning
		case <-time.After(1 * time.Second):
			log.Trace("checking vm state")
			state, _, err := dom.GetState()
			if err != nil {
				return nil, err
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

	// all is OK, commit (= no defer)
	commit = true
	return vm, nil
}
