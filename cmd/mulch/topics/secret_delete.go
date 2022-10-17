package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// secretDeleteCmd represents the "secret delete" command
var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name> <value>",
	Short: "Delete a secret value",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/secret/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	secretCmd.AddCommand(secretDeleteCmd)
}
