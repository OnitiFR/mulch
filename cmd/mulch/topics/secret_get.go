package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// secretGetCmd represents the "secret get" command
var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a secret value",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/secret/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	secretCmd.AddCommand(secretGetCmd)
}
