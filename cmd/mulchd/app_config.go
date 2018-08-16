package main

// AppConfig describes the general configuration of an App
type AppConfig struct {
	Listen     string
	LibVirtURI string
	// translated to a absolute local path (so libvirtd shound run next to us, currently)
	StoragePath string
	DataPath    string
	VMPrefix    string
	configPath  string
}
