package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test <vm-name>",
	Short: "Server test call",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/test/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	// rootCmd.AddCommand(testCmd)
}
