package main

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
)

// Reverse Proxy Chaining modes
const (
	ChainModeNone   = 0
	ChainModeChild  = 1
	ChainModeParent = 2
)

// ChainPSKMinLength is the minimum length for PSK
const ChainPSKMinLength = 16

// AppConfig describes the general configuration of an App
type AppConfig struct {
	// persistent storage
	DataPath string

	// ACME directory server
	AcmeURL string

	// ACME for issued certificate alerts
	AcmeEmail string

	// Listen HTTP address
	HTTPAddress string

	// Listen HTTPS address
	HTTPSAddress string

	// Mulch-proxy is in charge of Mulchd HTTPS LE certificate generation,
	// since port 80/443 is needed for that.
	ListenHTTPSDomain string

	// Reverse Proxy Chaining mode
	ChainMode int

	// if parent: listing API URL
	// if child: parent API URL
	ChainParentURL *url.URL

	// child only: URL we will register to parent
	ChainChildURL *url.URL

	// Pre-Shared key for the chain
	ChainPSK string

	// Replace X-Forward-For header with remote address
	ForceXForwardFor bool

	// global mulchd configuration path
	configPath string
}

type tomlAppConfig struct {
	DataPath          string `toml:"data_path"`
	AcmeURL           string `toml:"proxy_acme_url"`
	AcmeEmail         string `toml:"proxy_acme_email"`
	HTTPAddress       string `toml:"proxy_listen_http"`
	HTTPSAddress      string `toml:"proxy_listen_https"`
	ListenHTTPSDomain string `toml:"listen_https_domain"`

	ChainMode        string `toml:"proxy_chain_mode"`
	ChainParentURL   string `toml:"proxy_chain_parent_url"`
	ChainChildURL    string `toml:"proxy_chain_child_url"`
	ChainPSK         string `toml:"proxy_chain_psk"`
	ForceXForwardFor bool   `toml:"proxy_force_x_forward_for"`
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
		DataPath:     "./var/data", // example: /var/lib/mulch
		AcmeURL:      "https://acme-staging.api.letsencrypt.org/directory",
		AcmeEmail:    "root@localhost.localdomain",
		HTTPAddress:  ":80",
		HTTPSAddress: ":443",
	}

	meta, err := toml.DecodeFile(filename, tConfig)

	if err != nil {
		return nil, err
	}

	undecoded := meta.Undecoded()
	for _, param := range undecoded {
		if !strings.HasPrefix(param.String(), "proxy_") {
			continue
		}
		if param.String() == "proxy_listen_ssh" {
			continue
		}
		if param.String() == "proxy_ssh_extra_keys_file" {
			continue
		}
		return nil, fmt.Errorf("unknown setting '%s'", param)
	}

	appConfig.DataPath = tConfig.DataPath

	appConfig.AcmeURL = tConfig.AcmeURL
	if appConfig.AcmeURL == LEProductionString {
		appConfig.AcmeURL = "" // acme package default is production directory
	}
	appConfig.AcmeEmail = tConfig.AcmeEmail
	appConfig.HTTPAddress = tConfig.HTTPAddress
	appConfig.HTTPSAddress = tConfig.HTTPSAddress

	appConfig.ListenHTTPSDomain = tConfig.ListenHTTPSDomain

	switch tConfig.ChainMode {
	case "":
		appConfig.ChainMode = ChainModeNone
	case "child":
		appConfig.ChainMode = ChainModeChild
	case "parent":
		appConfig.ChainMode = ChainModeParent
	default:
		return nil, fmt.Errorf("unknown proxy_chain_mode value '%s'", tConfig.ChainMode)
	}

	appConfig.ChainPSK = tConfig.ChainPSK

	if appConfig.ChainMode != ChainModeNone {
		if tConfig.ChainParentURL == "" {
			return nil, errors.New("proxy_chain_parent_url is required for proxy chaining")
		}
		appConfig.ChainParentURL, err = url.ParseRequestURI(tConfig.ChainParentURL)
		if err != nil {
			return nil, errors.New("proxy_chain_parent_url is invalid")
		}

		if appConfig.ChainPSK == "" {
			return nil, errors.New("proxy_chain_psk is required for proxy chaining")
		}
		if len(appConfig.ChainPSK) < ChainPSKMinLength {
			return nil, fmt.Errorf("proxy_chain_psk is too short (min length = %d)", ChainPSKMinLength)
		}
	}

	if appConfig.ChainMode == ChainModeChild {
		if tConfig.ChainChildURL == "" {
			return nil, errors.New("proxy_chain_child_url is required for proxy chain children")
		}
		appConfig.ChainChildURL, err = url.ParseRequestURI(tConfig.ChainChildURL)
		if err != nil {
			return nil, errors.New("proxy_chain_child_url is invalid")
		}
	}

	if appConfig.ChainMode == ChainModeParent {
		if tConfig.ChainChildURL != "" {
			return nil, errors.New("proxy_chain_child_url is reserved for proxy chain children")
		}
	}

	appConfig.ForceXForwardFor = tConfig.ForceXForwardFor

	return appConfig, nil
}
