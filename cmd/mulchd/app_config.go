package main

// AppConfig describes the general configuration of an App
type AppConfig struct {
	// address where the API server will listen
	Listen string

	// URI to libvirtd (qemu only, currently)
	LibVirtURI string

	// translated to a absolute local path (so libvirtd shound run next to us, currently)
	StoragePath string

	// persistent storage (ex: VM database)
	// TODO: create path if needed on startup
	DataPath string

	// prefix for VM names (in libvirt)
	VMPrefix string

	// SSH keys used by Mulch to control & command VMs
	// TODO: check files on startup? (warning? error?)
	MulchSSHPrivateKey string
	MulchSSHPublicKey  string

	// global mulchd configuration path
	configPath string
}
