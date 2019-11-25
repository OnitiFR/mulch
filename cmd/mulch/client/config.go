package client

// RootConfig describes client application config parameters
type RootConfig struct {
	ConfigFile string

	Server  *ServerConfig
	Aliases map[string]string
	Trace   bool
	Time    bool
}

// ServerConfig describes a server (from config file)
// Warning: this structure is copied field by field in
// root_config.go, NewRootConfig(), so remember to update
// this function if any change is made here.
type ServerConfig struct {
	Name  string
	URL   string
	Key   string
	Alias string
}

// GlobalHome is the user HOME
var GlobalHome string

// GlobalCfgFile is the config filename
var GlobalCfgFile string

// GlobalAPI is the global API instance
var GlobalAPI *API

// GlobalConfig is the global RootConfig instance
var GlobalConfig *RootConfig
