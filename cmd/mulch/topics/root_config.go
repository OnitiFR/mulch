package topics

import (
    "os"
    "strconv"

    "github.com/BurntSushi/toml"
)

// RootConfig describes client application config parameters
type RootConfig struct {
	ConfigFile string

	URL   string
	Trace bool
	Time  bool
}

type tomlRootConfig struct {
	URL   string
	Trace bool
	Time  bool
}

// NewRootConfig reads configuration from filename and
// environment.
// Priority : CLI flag, config file, environment
func NewRootConfig(filename string) (*RootConfig, error) {
	rootConfig := &RootConfig{}

    envTrace, _ := strconv.ParseBool(os.Getenv("TRACE"))
    envTime, _ := strconv.ParseBool(os.Getenv("TIME"))

	tConfig := &tomlRootConfig{
        URL: os.Getenv("URL"),
        Trace: envTrace,
        Time: envTime,
    }

    if _, err := os.Stat(filename); err == nil {
        if _, err := toml.DecodeFile(filename, tConfig); err != nil {
    		return nil, err
    	}
        rootConfig.ConfigFile = filename
    }

    flagURL := rootCmd.PersistentFlags().Lookup("url")
    flagTrace := rootCmd.PersistentFlags().Lookup("trace")
    flagTime := rootCmd.PersistentFlags().Lookup("time")

    if flagURL.Changed {
        tConfig.URL = flagURL.Value.String()
    }
    if tConfig.URL == "" {
        tConfig.URL = flagURL.DefValue
    }

    if flagTrace.Changed {
        trace ,_ := strconv.ParseBool(flagTrace.Value.String())
        tConfig.Trace = trace
    }
    if flagTime.Changed {
        time, _ := strconv.ParseBool(flagTime.Value.String())
        tConfig.Time = time
    }

    rootConfig.URL = tConfig.URL
    rootConfig.Trace = tConfig.Trace
    rootConfig.Time = tConfig.Time

	return rootConfig, nil
}
