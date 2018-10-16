package topics

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/Xfennec/mulch/cmd/mulch/client"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var globalHome string
var globalCfgFile string

var globalAPI *client.API
var globalConfig *RootConfig

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mulch",
	Short: "Mulch CLI client",
	Long: `Mulch is a light and practical virtual machine manager, using
libvirt API. This is the client.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s\n\n", cmd.Short)
		fmt.Printf("%s\n\n", cmd.Long)
		fmt.Printf("Use --help to list commands and options.\n\n")
		if globalConfig.ConfigFile != "" {
			fmt.Printf("configuration file '%s', server '%s'\n",
				globalConfig.ConfigFile,
				globalConfig.Server.Name,
			)
		} else {
			fmt.Printf(`No configuration file found (%s).

Example:
[[server]]
name = "my-mulch"
url = "http://192.168.10.104:8585"
key = "gein2xah7keeL33thpe9ahvaegF15TUL3surae3Chue4riokooJ5WuTI80FTWfz2"

You can define multiple servers and use -s option to select one, or use
default = "my-mulch" as a global setting (i.e. before [[server]]).
First server is the default.

Global settings: trace, time
Note: you can also use environment variables (TRACE, TIME, SERVER).
------
`, path.Clean(globalHome+"/.mulch.toml"))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	var err error
	globalHome, err = homedir.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err = rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&globalCfgFile, "config", "c", "", "config file (default is $HOME/.mulch.toml)")

	rootCmd.PersistentFlags().BoolP("trace", "t", false, "also show server TRACE messages (debug)")
	rootCmd.PersistentFlags().BoolP("time", "d", false, "show server timestamps on messages")
	rootCmd.PersistentFlags().StringP("server", "s", "", "selected server in the config file")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	cfgFile := globalCfgFile
	if cfgFile == "" {
		cfgFile = path.Clean(globalHome + "/.mulch.toml")
	}

	var err error
	globalConfig, err = NewRootConfig(cfgFile)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	globalAPI = client.NewAPI(
		globalConfig.Server.URL,
		globalConfig.Server.Key,
		globalConfig.Trace,
		globalConfig.Time,
	)
}
