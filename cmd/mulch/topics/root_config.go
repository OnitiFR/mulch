package topics

import (
	"fmt"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

// RootConfig describes client application config parameters
type RootConfig struct {
	ConfigFile string

	Server  *tomlServerConfig
	Aliases map[string]string
	Trace   bool
	Time    bool
}

type tomlServerConfig struct {
	Name  string
	URL   string
	Key   string
	Alias string
}

type tomlRootConfig struct {
	Trace   bool
	Time    bool
	Default string
	Server  []*tomlServerConfig
}

// NewRootConfig reads configuration from filename and
// environment.
// Priority : CLI flag, config file, environment
func NewRootConfig(filename string) (*RootConfig, error) {
	rootConfig := &RootConfig{}

	envTrace, _ := strconv.ParseBool(os.Getenv("TRACE"))
	envTime, _ := strconv.ParseBool(os.Getenv("TIME"))
	envServer := os.Getenv("SERVER")

	tConfig := &tomlRootConfig{
		Trace:   envTrace,
		Time:    envTime,
		Default: envServer,
	}

	if stat, err := os.Stat(filename); err == nil {

		requiredMode, err := strconv.ParseInt("0600", 8, 32)
		if err != nil {
			return nil, err
		}

		if stat.Mode() != os.FileMode(requiredMode) {
			return nil, fmt.Errorf("%s: only the owner should be able to read/write this file (chmod 0600 %s)", filename, filename)
		}

		if _, err := toml.DecodeFile(filename, tConfig); err != nil {
			return nil, err
		}
		rootConfig.ConfigFile = filename
	} else {
		return nil, nil
	}

	flagTrace := rootCmd.PersistentFlags().Lookup("trace")
	flagTime := rootCmd.PersistentFlags().Lookup("time")
	flagServer := rootCmd.PersistentFlags().Lookup("server")

	if flagTrace.Changed {
		trace, _ := strconv.ParseBool(flagTrace.Value.String())
		tConfig.Trace = trace
	}
	if flagTime.Changed {
		time, _ := strconv.ParseBool(flagTime.Value.String())
		tConfig.Time = time
	}

	if flagServer.Changed {
		tConfig.Default = flagServer.Value.String()
	}

	if len(tConfig.Server) == 0 {
		return nil, fmt.Errorf("must define at least one [[server]] in configuration file")
	}

	if tConfig.Default == "" {
		tConfig.Default = tConfig.Server[0].Name
	}

	for _, server := range tConfig.Server {
		if server.Name == tConfig.Default {
			if rootConfig.Server != nil {
				return nil, fmt.Errorf("multiple declaration of server '%s'", server.Name)
			}
			rootConfig.Server = server
		}
	}

	if rootConfig.Server == nil {
		return nil, fmt.Errorf("unable to find server '%s' in configuration file", tConfig.Default)
	}

	rootConfig.Aliases = make(map[string]string)
	for _, server := range tConfig.Server {
		if server.Alias != "" {
			_, exists := rootConfig.Aliases[server.Alias]
			if exists {
				return nil, fmt.Errorf("multiple declaration of alias '%s'", server.Alias)
			}
			rootConfig.Aliases[server.Alias] = server.Name
		}
	}

	rootConfig.Trace = tConfig.Trace
	rootConfig.Time = tConfig.Time

	return rootConfig, nil
}
