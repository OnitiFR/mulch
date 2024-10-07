package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyDeleteCmd represents the "key delete" command
var keyDeleteCmd = &cobra.Command{
	Use:   "delete <key-comment>",
	Short: "Delete an API key",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/key/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	keyCmd.AddCommand(keyDeleteCmd)
}
