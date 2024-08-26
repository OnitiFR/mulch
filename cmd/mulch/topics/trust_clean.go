package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// trustCleanCmd represents the "trust remove" command
var trustCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all forwarded keys for deleted and inactive VMs",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/key/trust/clean", map[string]string{})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustCleanCmd)
}
