package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var test2Cmd = &cobra.Command{
	Use:   "test2",
	Short: "Server test call (2)",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/test2", map[string]string{})
		call.Do()
	},
}

func init() {
	// rootCmd.AddCommand(test2Cmd)
}
