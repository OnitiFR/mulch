package topics

import (
	"github.com/Xfennec/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "display server logs",
	Long: `Display all logs from the server. It may be useful to monitor
server activity, or if you need to resume VM creation after exiting
the client. All logs from all targets ("vm") are displayed.

Examples:
  mulch log
  mulch log --trace`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		trace, _ := cmd.Flags().GetBool("trace")
		api := client.NewAPI(viper.GetString("url"), trace)
		call := api.NewCall("GET", "/log", map[string]string{})
		call.Do()
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().BoolP("trace", "t", false, "also show TRACE messages (debug)")
}
