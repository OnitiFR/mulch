package main

import (
	"path"

	"github.com/BurntSushi/toml"
)

// TODO: add autocert email address
// TODO: add staging / production certificates setting

// AppConfig describes the general configuration of an App
type AppConfig struct {
	// persistent storage
	DataPath string

	// global mulchd configuration path
	configPath string
}

type tomlAppConfig struct {
	DataPath string `toml:"data_path"`
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
		DataPath: "./var/data", // example: /var/lib/mulch
	}

	if _, err := toml.DecodeFile(filename, tConfig); err != nil {
		return nil, err
	}

	appConfig.DataPath = tConfig.DataPath

	return appConfig, nil
}
