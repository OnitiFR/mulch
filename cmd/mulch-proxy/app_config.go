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

	// Rate controller configuration (if any)
	RateControllerConfig *RateControllerConfig

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

	RateConcurrentMaxRequests     int32    `toml:"proxy_rate_concurrent_max_requests"`
	RateConcurrentOverflowTimeout float64  `toml:"proxy_rate_concurrent_overflow_timeout_seconds"`
	RateLimitBurstRequests        int      `toml:"proxy_rate_limit_burst_requests"`
	RateLimitRequestsPerSecond    float64  `toml:"proxy_rate_limit_requests_per_second"`
	RateLimitMaxDelaySeconds      float64  `toml:"proxy_rate_limit_max_delay_seconds"`
	RateVIPList                   []string `toml:"proxy_rate_vip_list"`
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

	// concurrent requests
	if tConfig.RateConcurrentMaxRequests > 0 {
		appConfig.RateControllerConfig = &RateControllerConfig{
			ConcurrentMaxRequests:     tConfig.RateConcurrentMaxRequests,
			ConcurrentOverflowTimeout: time.Duration(tConfig.RateConcurrentOverflowTimeout * float64(time.Second)),
		}
	} else {
		if tConfig.RateConcurrentOverflowTimeout > 0 {
			return nil, errors.New("proxy_rate_concurrent_overflow_timeout requires proxy_rate_concurrent_max_requests")
		}
	}

	// rate limit
	if tConfig.RateLimitBurstRequests > 0 || tConfig.RateLimitRequestsPerSecond > 0 || tConfig.RateLimitMaxDelaySeconds > 0 {
		if tConfig.RateLimitBurstRequests <= 0 {
			return nil, errors.New("proxy_rate_limit_burst_requests must be > 0")
		}

		if tConfig.RateLimitRequestsPerSecond <= 1 {
			return nil, errors.New("proxy_rate_limit_requests_per_second must be > 1")
		}

		if appConfig.RateControllerConfig == nil {
			appConfig.RateControllerConfig = &RateControllerConfig{}
		}
		appConfig.RateControllerConfig.RateEnable = true
		appConfig.RateControllerConfig.RateBurst = tConfig.RateLimitBurstRequests
		appConfig.RateControllerConfig.RateRequestsPerSecond = tConfig.RateLimitRequestsPerSecond
		appConfig.RateControllerConfig.RateMaxDelay = time.Duration(tConfig.RateLimitMaxDelaySeconds * float64(time.Second))
	}

	if len(tConfig.RateVIPList) > 0 {
		if appConfig.RateControllerConfig == nil {
			return nil, errors.New("proxy_rate_vip_list requires a rate limit configuration")
		}
		appConfig.RateControllerConfig.VipList = make(map[string]bool)
		for _, ip := range tConfig.RateVIPList {
			if _, ok := appConfig.RateControllerConfig.VipList[ip]; ok {
				return nil, fmt.Errorf("duplicate IP in proxy_rate_vip_list: %s", ip)
			}

			appConfig.RateControllerConfig.VipList[ip] = true
		}
	}

	return appConfig, nil
}
