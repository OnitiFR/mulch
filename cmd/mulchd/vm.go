package main

import (
	"fmt"
	"io/ioutil"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

// VM defines a virtual machine ("domain")
type VM struct {
	UUID   string
	App    *App
	Config *VMConfig
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

	vm := &VM{
		App:    app,
		Config: vmConfig, // copy()? (deep)
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
			log.Infof("rollback, deleting '%s'", diskName)
			vol, errDef := app.Libvirt.Pools.Disks.LookupStorageVolByName(diskName)
			if errDef != nil {
				return
			}
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
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
		vmConfig.Name, // using this as instance-id, currently
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
			log.Infof("rollback, deleting '%s'", ciName)
			vol, errDef := app.Libvirt.Pools.CloudInit.LookupStorageVolByName(ciName)
			if errDef != nil {
				return
			}
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				return
			}
		}
	}()

	// 4 - define domain
	// should dynamically define:
	// - name
	// - CPU count, RAM amount
	// - CPU topology
	// - main qcow2 disk path
	// - cloud init disk path
	// - bridge interface name
	// - interface MAC address
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
	// fmt.Println(domcfg2.Memory, domcfg2.CurrentMemory, domcfg2.Devices.Interfaces)

	domcfg.Name = domainName

	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-disk" {
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
		}
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-cloudinit" {
			disk.Source.File.File = app.Libvirt.Pools.CloudInitXML.Target.Path + "/" + ciName
		}
	}

	for _, intf := range domcfg.Devices.Interfaces {
		fmt.Println(intf.Source.Bridge.Bridge) // change this to mulch net Bridge
		fmt.Println(intf.MAC.Address)          // randomize that
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

	uuid, err := dom.GetUUIDString()
	if err != nil {
		return nil, err
	}
	vm.UUID = uuid

	// if true {
	// 	return nil, errors.New("Intentional error, for test")
	// }

	log.Infof("starting vm for cloud-init step")
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	fmt.Println(dom.GetState())

	// all is OK, commit (= no defer)
	commit = true
	return vm, nil
}
