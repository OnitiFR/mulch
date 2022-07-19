package topics

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mulch",
	Short: "Mulch CLI client",
	Long: `Mulch is a light and practical virtual machine manager, using
libvirt API. This is the client.`,
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Printf("%s\n\n", cmd.Short)
		fmt.Printf("%s\n\n", cmd.Long)
		fmt.Printf("Use --help to list commands and options.\n\n")
		fmt.Printf("configuration file '%s', server '%s'\n",
			client.GlobalConfig.ConfigFile,
			client.GlobalConfig.Server.Name,
		)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	var err error
	client.GlobalHome, err = homedir.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	if err = rootCmd.Execute(); err != nil {
		return err
	}
	return nil
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&client.GlobalCfgFile, "config", "c", "", "config file (default is $HOME/.mulch.toml)")

	rootCmd.PersistentFlags().BoolP("trace", "t", false, "also show server TRACE messages (debug)")
	rootCmd.PersistentFlags().CountP("time", "d", "show server time on messages (use -dd to also show date)")
	rootCmd.PersistentFlags().StringP("server", "s", "", "selected server in the config file")
	rootCmd.PersistentFlags().BoolP("dump-servers", "", false, "dump server list and exit")

	rootCmd.PersistentFlags().BoolP("dump-server", "", false, "dump current server name (useful for completion)")
	rootCmd.PersistentFlags().MarkHidden("dump-server")

	// since MarkPersistentFlagCustom does not exists:
	serverFlagAnnotation := make(map[string][]string)
	serverFlagAnnotation[cobra.BashCompCustom] = []string{"__mulch_get_servers"}
	rootCmd.PersistentFlags().Lookup("server").Annotations = serverFlagAnnotation
}

func setCompletion() {
	aliases := ""
	for alias, server := range client.GlobalConfig.Aliases {
		aliases += fmt.Sprintf("%s() { mulch -s %s \"$@\"; }\n", alias, server)
		aliases += fmt.Sprintf("complete -o default -F __start_mulch %s\n", alias)
	}
	rootCmd.BashCompletionFunction = aliases + "\n" + bashCompletionFunc
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	cfgFile := client.GlobalCfgFile
	if cfgFile == "" {
		cfgFile = path.Clean(client.GlobalHome + "/.mulch.toml")
	}

	var err error
	client.GlobalConfig, err = NewRootConfig(cfgFile)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	if client.GlobalConfig == nil {
		fmt.Printf(`ERROR: Configuration file not found: %s

Example:

[[server]]
name = "my-mulch"
url = "http://192.168.10.104:8686"
key = "gein2xah7keeL33thpe9ahvaegF15TUL3surae3Chue4riokooJ5WuTI80FTWfz2"
alias = "my"

You can define multiple servers and use -s option to select one, or use
default = "my-mulch" as a global setting (i.e. before [[server]]).
First server is the default.

Alias is optionnal but cool, see 'mulch completion' for informations.

Global settings: trace, timestamp (values: time/datetime)
Note: you can also use environment variables (SERVER, TRACE, TIMESTAMP).
`, cfgFile)
		os.Exit(1)
	}

	client.GlobalAPI = client.NewAPI(
		client.GlobalConfig.Server.URL,
		client.GlobalConfig.Server.Key,
		client.GlobalConfig.Trace,
		client.GlobalConfig.Time,
	)

	setCompletion()

	if rootCmd.PersistentFlags().Lookup("dump-server").Changed {
		fmt.Println(client.GlobalConfig.Server.Name)
		os.Exit(0)
	}

}
