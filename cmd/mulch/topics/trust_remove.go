package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// trustRemoveCmd represents the "trust remove" command
var trustRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove a trusted VM from my key",
	Aliases: []string{"delete"},
	Args:    cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/key/trust/list/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustRemoveCmd)
}
