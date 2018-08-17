package main

import (
	"fmt"

	"github.com/libvirt/libvirt-go"
)

// VM defines a virtual machine ("domain")
type VM struct {
	UUID   string
	App    *App
	Config *VMConfig
}

// VMConfig stores needed parameters for a new VM
type VMConfig struct {
	Name           string
	ReferenceImage string // "FromImage"? "BaseImage"?
	DiskSize       uint64
	RAMSize        uint64
	CPUCount       int
	// SSH key? (or is it a global setting?) (filename? priv/pub content?)
	// + prepare scripts
	// + save scripts
	// + restore scripts
}

// NewVM builds a new virtual machine from config
func NewVM(vmConfig *VMConfig, app *App, log *Log) (*VM, error) {
	commit := false

	vm := &VM{
		App:    app,
		Config: vmConfig, // copy()? (deep)
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	_, err = conn.LookupDomainByName(app.Config.VMPrefix + vmConfig.Name)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists", vmConfig.Name)
	}
	errDetails := err.(libvirt.Error)
	if errDetails.Domain != libvirt.FROM_QEMU || errDetails.Code != libvirt.ERR_NO_DOMAIN {
		return nil, fmt.Errorf("Unexpected error: %s", err)
	}

	diskName := app.Config.VMPrefix + vmConfig.Name + ".qcow2"

	// 1 - copy from reference image
	log.Infof("creating VM disk '%s'", diskName)
	err = app.Libvirt.CreateDiskFromSeed(
		vmConfig.ReferenceImage,
		diskName,
		app.Config.configPath+"/templates/volume.xml",
		app.Log)

	if err != nil {
		return nil, err
	}

	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			vol, err := app.Libvirt.Pools.Disks.LookupStorageVolByName(diskName)
			if err != nil {
				return
			}
			err = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if err != nil {
				return
			}
		}
	}()

	// if true {
	// 	return nil, errors.New("Intentional error, for test")
	// }

	// all is OK, commit (= no defer)
	commit = true
	return vm, nil
}
