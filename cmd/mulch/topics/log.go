package topics

import (
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display server logs",
	Long: `Display all logs from the server. It may be useful to monitor
server activity, or if you need to resume VM creation after exiting
the client. All logs from all targets ("vm") are displayed.

Examples:
  mulch log
  mulch log --trace`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// trace, _ := cmd.Flags().GetBool("trace")
		// api := client.NewAPI(viper.GetString("url"), viper.GetBool("trace"))
		call := globalAPI.NewCall("GET", "/log", map[string]string{})
		call.Do()
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	// logCmd.Flags().BoolP("trace", "t", false, "also show TRACE messages (debug)")
}
