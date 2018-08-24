package topics

import (
	"fmt"
	"os"
	"strings"

	"github.com/Xfennec/mulch/cmd/mulch/client"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var globalCfgFile string
var globalHome string
var globalAPI *client.API

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mulch",
	Short: "Mulch CLI client",
	Long: `Mulch is a light and practical virtual machine manager, using
libvirt API. This is the client.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s\n\n", cmd.Short)
		fmt.Printf("%s\n\n", cmd.Long)

		cfgFile := viper.ConfigFileUsed()
		if cfgFile != "" {
			fmt.Printf("configuration file: '%s'\n", cfgFile)
		} else {
			fmt.Printf(`No configuration file found, you can create one in '%s'
with the name '.mulch.xxx' using one of the following
formats (replace xxx with the right extension of the list):
 - %s

This config file must provide 'key' setting (with your API
key) and probably the mulchd API URL with the 'url' setting.

Example: (~/.mulch.toml)
url = "http://192.168.10.104"
key = "gein2xah7keel5Ohpe9ahvaeg8suurae3Chue4riokooJ5Wu"

Available settings: trace, timestamps
Note: you can also use environment variables (URL, KEY, …).
------
`, globalHome, strings.Join(viper.SupportedExts, ", "))
		}
		fmt.Printf("current URL to mulchd: %s\n", viper.GetString("url"))
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&globalCfgFile, "config", "", "config file (default is $HOME/.mulch.yaml)")

	rootCmd.PersistentFlags().StringP("url", "u", "http://localhost:8585", "mulchd URL (default is http://localhost:8585)")
	rootCmd.PersistentFlags().BoolP("trace", "t", false, "also show server TRACE messages (debug)")
	rootCmd.PersistentFlags().BoolP("time", "d", false, "show server timestamps on messages")

	viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	viper.BindPFlag("trace", rootCmd.PersistentFlags().Lookup("trace"))
	viper.BindPFlag("time", rootCmd.PersistentFlags().Lookup("time"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if globalCfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(globalCfgFile)
	} else {
		// Find home directory.
		var err error
		globalHome, err = homedir.Dir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		// Search config in home directory with name ".mulch" (without extension).
		viper.AddConfigPath(globalHome)
		viper.SetConfigName(".mulch")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		// fmt.Println("Using config file:", viper.ConfigFileUsed())
	} else {
		_, ok := err.(viper.ConfigFileNotFoundError)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error while eading '%s':\n", viper.ConfigFileUsed())
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	// Init global vars
	globalAPI = client.NewAPI(viper.GetString("url"), viper.GetBool("trace"))
	// fmt.Printf("Server: %s\n", globalAPI.ServerURL)
}
