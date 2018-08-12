package main

// AppConfig describes the general configuration of an App
type AppConfig struct {
	Listen      string
	LibVirtURI  string
	StoragePath string

	configPath string
}
