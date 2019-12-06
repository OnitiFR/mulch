package server

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// AppConfig describes the general configuration of an App
type AppConfig struct {
	// address where the API server will listen
	Listen string

	// API server HTTPS domain name (HTTP otherwise)
	ListenHTTPSDomain string

	// URI to libvirtd (qemu only, currently)
	LibVirtURI string

	// translated to a absolute local path (so libvirtd shound run next to us, currently)
	StoragePath string

	// persistent storage (ex: VM database)
	// TODO: create path if needed on startup
	DataPath string

	// temporary files path (ioutil.TempFile)
	TempPath string

	// prefix for VM names (in libvirt)
	VMPrefix string

	// SSH proxy listen address
	ProxyListenSSH string

	// Extra (limited) SSH keys
	ProxySSHExtraKeysFile string

	// User (sudoer) created by Mulch in VMs
	MulchSuperUser string

	// Everyday VM auto-rebuild time ("HH:MM")
	AutoRebuildTime string

	// Seeds
	Seeds map[string]ConfigSeed

	// global mulchd configuration path
	configPath string
}

// ConfigSeed describes a OS seed
type ConfigSeed struct {
	URL    string
	Seeder string
}

type tomlAppConfig struct {
	Listen                string
	ListenHTTPSDomain     string `toml:"listen_https_domain"`
	LibVirtURI            string `toml:"libvirt_uri"`
	StoragePath           string `toml:"storage_path"`
	DataPath              string `toml:"data_path"`
	TempPath              string `toml:"temp_path"`
	VMPrefix              string `toml:"vm_prefix"`
	ProxyListenSSH        string `toml:"proxy_listen_ssh"`
	ProxySSHExtraKeysFile string `toml:"proxy_ssh_extra_keys_file"`
	MulchSuperUser        string `toml:"mulch_super_user"`
	AutoRebuildTime       string `toml:"auto_rebuild_time"`
	Seed                  []tomlConfigSeed
}

type tomlConfigSeed struct {
	Name   string
	URL    string
	Seeder string
}

// NewAppConfigFromTomlFile return a AppConfig using
// mulchd.toml config file in the given configPath
func NewAppConfigFromTomlFile(configPath string) (*AppConfig, error) {

	filename := path.Clean(configPath + "/mulchd.toml")

	appConfig := &AppConfig{
		configPath: configPath,
		Seeds:      make(map[string]ConfigSeed),
	}

	// defaults (if not in the file)
	tConfig := &tomlAppConfig{
		Listen:                ":8686",
		LibVirtURI:            "qemu:///system",
		StoragePath:           "./var/storage", // example: /srv/mulch
		DataPath:              "./var/data",    // example: /var/lib/mulch
		TempPath:              "",
		VMPrefix:              "mulch-",
		ProxyListenSSH:        ":8022",
		ProxySSHExtraKeysFile: "",
		MulchSuperUser:        "admin",
		AutoRebuildTime:       "23:30",
	}

	if _, err := toml.DecodeFile(filename, tConfig); err != nil {
		return nil, err
	}

	partsL := strings.Split(tConfig.Listen, ":")
	if len(partsL) != 2 {
		return nil, fmt.Errorf("listen: '%s': wrong format (ex: ':8686')", tConfig.Listen)
	}

	listenPort, err := strconv.Atoi(partsL[1])
	if err != nil {
		return nil, fmt.Errorf("listen: '%s': wrong port number", tConfig.Listen)
	}

	if listenPort == AppInternalServerPost {
		return nil, fmt.Errorf("listen address '%s' is reserved for internal use", tConfig.Listen)
	}
	appConfig.Listen = tConfig.Listen
	appConfig.ListenHTTPSDomain = tConfig.ListenHTTPSDomain

	// no check here for most of config elements, it's done later
	appConfig.LibVirtURI = tConfig.LibVirtURI
	appConfig.StoragePath = tConfig.StoragePath
	appConfig.DataPath = tConfig.DataPath
	appConfig.TempPath = tConfig.TempPath
	appConfig.VMPrefix = tConfig.VMPrefix
	appConfig.MulchSuperUser = tConfig.MulchSuperUser

	appConfig.ProxyListenSSH = tConfig.ProxyListenSSH
	appConfig.ProxySSHExtraKeysFile = tConfig.ProxySSHExtraKeysFile

	partsAr := strings.Split(tConfig.AutoRebuildTime, ":")
	if len(partsAr) != 2 {
		return nil, fmt.Errorf("auto_rebuild_time: '%s': wrong format (HH:MM needed)", tConfig.AutoRebuildTime)
	}
	hour, err := strconv.Atoi(partsAr[0])
	if err != nil || hour > 23 || hour < 0 {
		return nil, fmt.Errorf("auto_rebuild_time: '%s': invalid hour", tConfig.AutoRebuildTime)
	}
	minute, err := strconv.Atoi(partsAr[1])
	if err != nil || minute > 59 || minute < 0 {
		return nil, fmt.Errorf("auto_rebuild_time: '%s': invalid minute", tConfig.AutoRebuildTime)
	}
	appConfig.AutoRebuildTime = tConfig.AutoRebuildTime

	for _, seed := range tConfig.Seed {
		if seed.Name == "" {
			return nil, fmt.Errorf("seed 'name' not defined")
		}

		if IsValidName(seed.Name) == false {
			return nil, fmt.Errorf("'%s' is not a valid seed name", seed.Name)
		}

		_, exists := appConfig.Seeds[seed.Name]
		if exists == true {
			return nil, fmt.Errorf("seed name '%s' already defined", seed.Name)
		}

		if (seed.URL == "" && seed.Seeder == "") ||
			(seed.URL != "" && seed.Seeder != "") {
			return nil, fmt.Errorf("seed '%s': must have either 'url' or 'seeder' parameter", seed.Name)
		}

		appConfig.Seeds[seed.Name] = ConfigSeed{
			URL:    seed.URL,
			Seeder: seed.Seeder,
		}

	}

	return appConfig, nil
}

// GetTemplateFilepath returns a path to a etc/template file
func (conf *AppConfig) GetTemplateFilepath(name string) string {
	return path.Clean(conf.configPath + "/templates/" + name)
}
