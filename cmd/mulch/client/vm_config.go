package client

import (
	"github.com/BurntSushi/toml"
)

// we only read one field, currently
type VMConfig struct {
	Name string
}

// NewVMConfigFromFile creates a new VMConfig from a TOML file
func NewVMConfigFromFile(filename string) (*VMConfig, error) {
	var tConfig VMConfig

	_, err := toml.DecodeFile(filename, &tConfig)

	if err != nil {
		return nil, err
	}

	return &tConfig, nil
}
