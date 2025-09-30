package main

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

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

	// Replace X-Forwarded-For header with remote address
	ForceXForwardedFor bool

	// Trusted third-party proxies
	TrustedProxies     map[string]bool
	HaveTrustedProxies bool

	// Rate controller configuration (if any)
	RateControllerConfigs map[string]*RateControllerConfig

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

	ChainMode      string `toml:"proxy_chain_mode"`
	ChainParentURL string `toml:"proxy_chain_parent_url"`
	ChainChildURL  string `toml:"proxy_chain_child_url"`
	ChainPSK       string `toml:"proxy_chain_psk"`

	ForceXForwardedFor bool `toml:"proxy_force_x_forwarded_for"`

	TrustedProxies []string `toml:"proxy_trusted_proxies"`

	RateControllers []tomlConfigRate `toml:"proxy_rate"`
}

type tomlConfigRate struct {
	Name                      string   `toml:"name"`
	ConcurrentMaxRequests     int32    `toml:"concurrent_max_requests"`
	ConcurrentOverflowTimeout float64  `toml:"concurrent_overflow_timeout_seconds"`
	LimitBurstRequests        int      `toml:"limit_burst_requests"`
	LimitRequestsPerSecond    float64  `toml:"limit_requests_per_second"`
	LimitMaxDelaySeconds      float64  `toml:"limit_max_delay_seconds"`
	VipList                   []string `toml:"vip_list"`
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
		AcmeURL:      "https://acme-staging-v02.api.letsencrypt.org/directory",
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

	appConfig.ForceXForwardedFor = tConfig.ForceXForwardedFor

	appConfig.TrustedProxies = make(map[string]bool)

	if len(tConfig.TrustedProxies) > 0 {
		appConfig.HaveTrustedProxies = true

		for _, ip := range tConfig.TrustedProxies {
			if _, ok := appConfig.TrustedProxies[ip]; ok {
				return nil, fmt.Errorf("duplicate IP in proxy_trusted_proxies: %s", ip)
			}
			appConfig.TrustedProxies[ip] = true
		}
	}

	appConfig.RateControllerConfigs = make(map[string]*RateControllerConfig)

	for _, rc := range tConfig.RateControllers {
		if rc.Name == "" {
			return nil, errors.New("a proxy_rate entry is missing the name")
		}

		if _, ok := appConfig.RateControllerConfigs[rc.Name]; ok {
			return nil, fmt.Errorf("duplicate proxy_rate name: '%s'", rc.Name)
		}

		appRc := &RateControllerConfig{
			Name:                      rc.Name,
			ConcurrentMaxRequests:     rc.ConcurrentMaxRequests,
			ConcurrentOverflowTimeout: time.Duration(rc.ConcurrentOverflowTimeout * float64(time.Second)),
			RateEnable:                false,
		}

		if rc.LimitBurstRequests > 0 || rc.LimitRequestsPerSecond > 0 || rc.LimitMaxDelaySeconds > 0 {
			if rc.LimitBurstRequests <= 0 {
				return nil, fmt.Errorf("proxy_rate '%s': limit_burst_requests must be > 0", rc.Name)
			}

			if rc.LimitRequestsPerSecond <= 1 {
				return nil, fmt.Errorf("proxy_rate '%s': limit_requests_per_second must be > 1", rc.Name)
			}

			if rc.LimitMaxDelaySeconds <= 0 {
				return nil, fmt.Errorf("proxy_rate '%s': limit_max_delay_seconds must be > 0", rc.Name)
			}

			appRc.RateEnable = true
			appRc.RateBurst = rc.LimitBurstRequests
			appRc.RateRequestsPerSecond = rc.LimitRequestsPerSecond
			appRc.RateMaxDelay = time.Duration(rc.LimitMaxDelaySeconds * float64(time.Second))
		}

		if len(rc.VipList) > 0 {
			appRc.VipList = make(map[string]bool)
			for _, ip := range rc.VipList {
				if _, ok := appRc.VipList[ip]; ok {
					return nil, fmt.Errorf("duplicate IP in proxy_rate '%s' vip_list: %s", rc.Name, ip)
				}

				appRc.VipList[ip] = true
			}
		}

		appConfig.RateControllerConfigs[rc.Name] = appRc
	}

	// create "default" (empty) rate controller if not already defined
	if _, ok := appConfig.RateControllerConfigs["default"]; !ok {
		appConfig.RateControllerConfigs["default"] = &RateControllerConfig{
			Name: "default",
		}
	}

	return appConfig, nil
}
