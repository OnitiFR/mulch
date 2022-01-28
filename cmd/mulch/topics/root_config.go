package topics

import (
	"fmt"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
)

type tomlServerConfig struct {
	Name  string
	URL   string
	Key   string
	Alias string
}

type tomlRootConfig struct {
	Trace     bool
	Timestamp string
	Default   string
	Server    []*tomlServerConfig
}

// NewRootConfig reads configuration from filename and
// environment.
// Priority : CLI flag, config file, environment
func NewRootConfig(filename string) (*client.RootConfig, error) {
	rootConfig := &client.RootConfig{}

	envTrace, _ := strconv.ParseBool(os.Getenv("TRACE"))
	envTimestamp := os.Getenv("TIMESTAMP")
	envServer := os.Getenv("SERVER")

	tConfig := &tomlRootConfig{
		Trace:     envTrace,
		Timestamp: envTimestamp,
		Default:   envServer,
	}

	if stat, err := os.Stat(filename); err == nil {

		requiredMode, err := strconv.ParseInt("0600", 8, 32)
		if err != nil {
			return nil, err
		}

		if stat.Mode() != os.FileMode(requiredMode) {
			return nil, fmt.Errorf("%s: only the owner should be able to read/write this file (chmod 0600 %s)", filename, filename)
		}

		meta, err := toml.DecodeFile(filename, tConfig)

		if err != nil {
			return nil, err
		}

		undecoded := meta.Undecoded()
		for _, param := range undecoded {
			return nil, fmt.Errorf("unknown setting '%s'", param)
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

	dCount, _ := strconv.Atoi(flagTime.Value.String())
	switch dCount {
	case 1:
		tConfig.Timestamp = "time"
	case 2:
		tConfig.Timestamp = "datetime"
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
			rootConfig.Server = &client.ServerConfig{
				Name:  server.Name,
				URL:   server.URL,
				Key:   server.Key,
				Alias: server.Alias,
			}
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
	rootConfig.Time = common.MessagePrintNoTime
	switch tConfig.Timestamp {
	case "time":
		rootConfig.Time = common.MessagePrintTime
	case "datetime":
		rootConfig.Time = common.MessagePrintDateTime
	}

	if rootCmd.PersistentFlags().Lookup("dump-servers").Changed {
		for _, server := range tConfig.Server {
			fmt.Println(server.Name)
		}
		os.Exit(0)
	}

	return rootConfig, nil
}
