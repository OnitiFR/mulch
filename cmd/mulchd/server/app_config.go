package server

import (
	"errors"
	"path"

	"github.com/BurntSushi/toml"
)

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
	MulchSSHPrivateKey string
	MulchSSHPublicKey  string

	// User (sudoer) created by Mulch in VMs
	MulchSuperUser string

	// global mulchd configuration path
	configPath string
}

type tomlAppConfig struct {
	Listen             string
	LibVirtURI         string `toml:"libvirt_uri"`
	StoragePath        string `toml:"storage_path"`
	DataPath           string `toml:"data_path"`
	VMPrefix           string `toml:"vm_prefix"`
	MulchSSHPrivateKey string `toml:"mulch_ssh_private_key"`
	MulchSSHPublicKey  string `toml:"mulch_ssh_public_key"`
	MulchSuperUser     string `toml:"mulch_super_user"`
}

// NewAppConfigFromTomlFile return a AppConfig using
// mulchd.toml config file in the given configPath
func NewAppConfigFromTomlFile(configPath string) (*AppConfig, error) {

	filename := path.Clean(configPath + "/mulchd.toml")

	appConfig := &AppConfig{
		configPath: configPath,
	}

	// defaults (if not in the file)
	tConfig := &tomlAppConfig{
		Listen:         ":8585",
		LibVirtURI:     "qemu:///system",
		StoragePath:    "./var/storage", // example: /srv/mulch
		DataPath:       "./var/data",    // example: /var/lib/mulch
		VMPrefix:       "mulch-",
		MulchSuperUser: "mulch-cc",
	}

	if _, err := toml.DecodeFile(filename, tConfig); err != nil {
		return nil, err
	}

	// no check here for most of config elements, it's done later
	appConfig.Listen = tConfig.Listen
	appConfig.LibVirtURI = tConfig.LibVirtURI
	appConfig.StoragePath = tConfig.StoragePath
	appConfig.DataPath = tConfig.DataPath
	appConfig.VMPrefix = tConfig.VMPrefix
	appConfig.MulchSuperUser = tConfig.MulchSuperUser

	if tConfig.MulchSSHPublicKey == "" {
		return nil, errors.New("'mulch_ssh_private_key' config param must be defined")
	}
	appConfig.MulchSSHPrivateKey = tConfig.MulchSSHPrivateKey

	if tConfig.MulchSSHPublicKey == "" {
		return nil, errors.New("'mulch_ssh_public_key' config param must be defined")
	}
	appConfig.MulchSSHPublicKey = tConfig.MulchSSHPublicKey

	return appConfig, nil
}
