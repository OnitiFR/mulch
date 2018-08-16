package main

import (
	"errors"
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

	// 1 - copy from reference image
	err = app.Libvirt.CreateDiskFromRelease(
		vmConfig.ReferenceImage,
		app.Config.VMPrefix+vmConfig.Name+".qcow2",
		app.Config.configPath+"/templates/volume.xml",
		app.Log)

	if err != nil {
		return nil, err
	}

	return nil, errors.New("Not implemented yet")
}
