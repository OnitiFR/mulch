package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// trustRemoveCmd represents the "trust remove" command
var trustRemoveCmd = &cobra.Command{
	Use:     "remove <vm> [ssh-key-fingerprint]",
	Short:   "Remove one or all SSH forwarded keys from a VM",
	Aliases: []string{"delete", "unforward"},
	Args:    cobra.RangeArgs(1, 2),
	Run: func(_ *cobra.Command, args []string) {
		fingerprint := ""
		if len(args) == 2 {
			fingerprint = args[1]
		}
		call := client.GlobalAPI.NewCall("DELETE", "/key/trust/list/"+args[0], map[string]string{
			"fingerprint": fingerprint,
		})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustRemoveCmd)
}
