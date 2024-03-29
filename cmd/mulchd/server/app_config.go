package server

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/OnitiFR/mulch/common"
)

// Reverse Proxy Chaining modes
const (
	ProxyChainModeNone   = 0
	ProxyChainModeChild  = 1
	ProxyChainModeParent = 2
)

const (
	OriginTypeHTTP = "http"
	OriginTypeGIT  = "git"
	OriginTypeFile = "file"
)

// AppConfig describes the general configuration of an App
type AppConfig struct {
	// address where the API server will listen
	Listen string

	// port for "phone home" internal HTTP server
	// (do not change if any VM was already built!)
	InternalServerPort int

	// API server HTTPS domain name (fallbacks to HTTP if not set)
	ListenHTTPSDomain string

	// URI to libvirtd (qemu only, currently)
	LibVirtURI string

	// local path (so libvirtd shound run next to us, currently)
	StoragePath string

	// persistent storage (ex: VM database)
	// TODO: create path if needed on startup
	DataPath string

	// temporary files path
	TempPath string

	// prefix for VM names (in libvirt)
	VMPrefix string

	// SSH proxy listen address
	ProxyListenSSH string

	// extra (limited) SSH keys
	ProxySSHExtraKeysFile string

	// reverse Proxy Chaining mode
	ProxyChainMode int

	// if parent: listening API URL
	// if child: parent API URL
	ProxyChainParentURL string

	// child only: URL we will register to parent
	ProxyChainChildURL string

	// pre-shared key for the chain
	ProxyChainPSK string

	// user (sudoer) created by Mulch in VMs
	MulchSuperUser string

	// name of the SSH key in SSHPairDatabase for this sudoer
	MulchSuperUserSSHKey string

	// everyday VM auto-rebuild time ("HH:MM")
	AutoRebuildTime string

	// seeds
	Seeds map[string]ConfigSeed

	// peers
	Peers map[string]ConfigPeer

	// origins
	Origins map[string]*ConfigOrigin

	// global mulchd configuration path
	configPath string
}

// ConfigSeed describes a OS seed
type ConfigSeed struct {
	URL    string
	Seeder string
}

// ConfigPeer describes a peer
type ConfigPeer struct {
	Name        string
	URL         string
	Key         string
	SyncSecrets bool
}

// ConfigOrigin describes an origin for scripts
type ConfigOrigin struct {
	Name       string
	Type       string
	Path       string
	Dir        string
	Branch     string
	SSHKeyFile string
	SSHAgent   bool
}

type tomlAppConfig struct {
	Listen                string
	InternalServerPort    int    `toml:"internal_port"`
	ListenHTTPSDomain     string `toml:"listen_https_domain"`
	LibVirtURI            string `toml:"libvirt_uri"`
	StoragePath           string `toml:"storage_path"`
	DataPath              string `toml:"data_path"`
	TempPath              string `toml:"temp_path"`
	VMPrefix              string `toml:"vm_prefix"`
	ProxyListenSSH        string `toml:"proxy_listen_ssh"`
	ProxySSHExtraKeysFile string `toml:"proxy_ssh_extra_keys_file"`
	ProxyChainMode        string `toml:"proxy_chain_mode"`
	ProxyChainParentURL   string `toml:"proxy_chain_parent_url"`
	ProxyChainChildURL    string `toml:"proxy_chain_child_url"`
	ProxyChainPSK         string `toml:"proxy_chain_psk"`
	MulchSuperUser        string `toml:"mulch_super_user"`
	MulchSuperUserSSHKey  string `toml:"mulch_super_user_ssh_key"`
	AutoRebuildTime       string `toml:"auto_rebuild_time"`
	Seed                  []tomlConfigSeed
	Peer                  []tomlConfigPeer
	Origin                []tomlConfigOrigin
}

type tomlConfigSeed struct {
	Name   string
	URL    string
	Seeder string
}

type tomlConfigPeer struct {
	Name        string
	URL         string
	Key         string
	SyncSecrets bool `toml:"sync_secrets"`
}

type tomlConfigOrigin struct {
	Name       string
	Type       string
	Path       string
	Dir        string
	Branch     string
	SSHKeyFile string `toml:"ssh_key_file"`
	SSHAgent   bool   `toml:"ssh_agent"`
}

// NewAppConfigFromTomlFile return a AppConfig using
// mulchd.toml config file in the given configPath
func NewAppConfigFromTomlFile(configPath string) (*AppConfig, error) {

	filename := path.Clean(configPath + "/mulchd.toml")

	appConfig := &AppConfig{
		configPath: configPath,
		Seeds:      make(map[string]ConfigSeed),
		Peers:      make(map[string]ConfigPeer),
		Origins:    make(map[string]*ConfigOrigin),
	}

	// defaults (if not in the file)
	tConfig := &tomlAppConfig{
		Listen:                ":8686",
		InternalServerPort:    8585,
		LibVirtURI:            "qemu:///system",
		StoragePath:           "./var/storage", // example: /srv/mulch
		DataPath:              "./var/data",    // example: /var/lib/mulch
		TempPath:              "",
		VMPrefix:              "mulch-",
		ProxyListenSSH:        ":8022",
		ProxySSHExtraKeysFile: "",
		MulchSuperUser:        "admin",
		MulchSuperUserSSHKey:  "mulch_super_user",
		AutoRebuildTime:       "23:30",
	}

	meta, err := toml.DecodeFile(filename, tConfig)

	if err != nil {
		return nil, err
	}

	undecoded := meta.Undecoded()
	for _, param := range undecoded {
		// this check is far from perfect, since we (mulchd) use
		// settings like proxy_listen_ssh and proxy_ssh_extra_keys_file…
		if strings.HasPrefix(param.String(), "proxy_") {
			continue
		}
		return nil, fmt.Errorf("unknown setting '%s'", param)
	}

	partsL := strings.Split(tConfig.Listen, ":")
	if len(partsL) != 2 {
		return nil, fmt.Errorf("listen: '%s': wrong format (ex: ':8686')", tConfig.Listen)
	}

	listenPort, err := strconv.Atoi(partsL[1])
	if err != nil {
		return nil, fmt.Errorf("listen: '%s': wrong port number", tConfig.Listen)
	}

	appConfig.Listen = tConfig.Listen
	appConfig.InternalServerPort = tConfig.InternalServerPort
	appConfig.ListenHTTPSDomain = tConfig.ListenHTTPSDomain

	if listenPort == appConfig.InternalServerPort {
		return nil, fmt.Errorf("listen address '%s' is reserved for internal_port", tConfig.Listen)
	}

	// no check here for most of config elements, it's done later
	appConfig.LibVirtURI = tConfig.LibVirtURI
	appConfig.StoragePath = tConfig.StoragePath
	appConfig.DataPath = tConfig.DataPath
	appConfig.TempPath = tConfig.TempPath
	appConfig.VMPrefix = tConfig.VMPrefix
	appConfig.MulchSuperUser = tConfig.MulchSuperUser
	appConfig.MulchSuperUserSSHKey = tConfig.MulchSuperUserSSHKey

	appConfig.ProxyListenSSH = tConfig.ProxyListenSSH
	appConfig.ProxySSHExtraKeysFile = tConfig.ProxySSHExtraKeysFile

	switch tConfig.ProxyChainMode {
	case "":
		appConfig.ProxyChainMode = ProxyChainModeNone
	case "child":
		appConfig.ProxyChainMode = ProxyChainModeChild
	case "parent":
		appConfig.ProxyChainMode = ProxyChainModeParent
	default:
		return nil, fmt.Errorf("unknown proxy_chain_mode value '%s'", tConfig.ProxyChainMode)
	}
	// no validation here, it's done by mulch-proxy, we're just an API client
	appConfig.ProxyChainParentURL = tConfig.ProxyChainParentURL
	appConfig.ProxyChainChildURL = tConfig.ProxyChainChildURL
	appConfig.ProxyChainPSK = tConfig.ProxyChainPSK

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

		if !IsValidName(seed.Name) {
			return nil, fmt.Errorf("'%s' is not a valid seed name", seed.Name)
		}

		_, exists := appConfig.Seeds[seed.Name]
		if exists {
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

	for _, peer := range tConfig.Peer {
		if peer.Name == "" {
			return nil, fmt.Errorf("peer 'name' not defined")
		}

		if !IsValidName(peer.Name) {
			return nil, fmt.Errorf("'%s' is not a valid peer name", peer.Name)
		}

		_, exists := appConfig.Peers[peer.Name]
		if exists {
			return nil, fmt.Errorf("duplicate peer '%s'", peer.Name)
		}

		if peer.URL == "" {
			return nil, fmt.Errorf("peer '%s' have undefined 'url'", peer.Name)
		}

		// IDEA: test URL + key and show warning in case of failure?

		appConfig.Peers[peer.Name] = ConfigPeer(peer)
	}

	for _, origin := range tConfig.Origin {
		if origin.Name == "" {
			return nil, fmt.Errorf("origin 'name' not defined")
		}

		if !IsValidName(origin.Name) {
			return nil, fmt.Errorf("'%s' is not a valid origin name", origin.Name)
		}

		_, exists := appConfig.Origins[origin.Name]
		if exists {
			return nil, fmt.Errorf("duplicate origin '%s'", origin.Name)
		}

		if origin.Path == "" {
			return nil, fmt.Errorf("origin 'path' not defined")
		}

		origConf := ConfigOrigin{
			Name: origin.Name,
			Path: origin.Path,
		}

		switch origin.Type {
		case "http":
			origConf.Type = OriginTypeHTTP
		case "git":
			origConf.Type = OriginTypeGIT
		case "file":
			origConf.Type = OriginTypeFile
		default:
			return nil, fmt.Errorf("origin '%s': unknown type '%s'", origin.Name, origin.Type)
		}

		if origConf.Type != OriginTypeGIT {
			if origin.Dir != "" {
				return nil, fmt.Errorf("origin '%s': 'dir' parameter is only valid for 'git' type", origin.Name)
			}
			if origin.Branch != "" {
				return nil, fmt.Errorf("origin '%s': 'branch' parameter is only valid for 'git' type", origin.Name)
			}
			if origin.SSHKeyFile != "" {
				return nil, fmt.Errorf("origin '%s': 'ssh_key_file' parameter is only valid for 'git' type", origin.Name)
			}
		} else {
			if origin.Branch == "" {
				return nil, fmt.Errorf("origin '%s': 'branch' parameter is required for 'git' type", origin.Name)
			}
			if origin.SSHKeyFile == "" && !origin.SSHAgent {
				return nil, fmt.Errorf("origin '%s': 'ssh_key_file' or 'ssh_agent = true' parameter is required for 'git' type", origin.Name)
			}
			if origin.SSHKeyFile != "" && origin.SSHAgent {
				return nil, fmt.Errorf("origin '%s': 'ssh_key_file' and 'ssh_agent = true' parameters are mutually exclusive", origin.Name)
			}
			if origin.SSHKeyFile != "" {
				if !common.PathExist(origin.SSHKeyFile) {
					return nil, fmt.Errorf("origin '%s': SSH key '%s' does not exist", origin.Name, origin.SSHKeyFile)
				}
			}

			origConf.Branch = origin.Branch
			origConf.SSHKeyFile = origin.SSHKeyFile
			origConf.SSHAgent = origin.SSHAgent
		}

		appConfig.Origins[origin.Name] = &origConf
	}

	return appConfig, nil
}

// GetTemplateFilepath returns a path to a etc/template file
func (conf *AppConfig) GetTemplateFilepath(name string) string {
	return path.Clean(conf.configPath + "/templates/" + name)
}
