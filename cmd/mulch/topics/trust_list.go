package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// trustListCmd represents the "trust list" command
var trustListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your trusted VMs",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/key/trust/list", map[string]string{})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustListCmd)
}
