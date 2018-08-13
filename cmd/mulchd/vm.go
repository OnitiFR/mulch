package main

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
	// + prepare scripts
	// + save scripts
	// + restore scripts
}

// NewVM builds a new virtual machine from config
func NewVM(vmConfig *VMConfig, app *App) (*VM, error) {
	return nil, nil
}
