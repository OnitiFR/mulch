package main

import "errors"

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
	return nil, errors.New("Not implemented yet")
}
