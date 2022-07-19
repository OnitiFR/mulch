package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyRightRemoveCmd represents the "key right remove" command
var keyRightRemoveCmd = &cobra.Command{
	Use:   "remove <key> <right>",
	Short: "Remove a right to the key",
	Long: `Remove a right to the key

WARNING: no rights means full privileges.
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/key/right/"+args[0], map[string]string{
			"right": args[1],
		})
		call.Do()
	},
}

func init() {
	keyRightCmd.AddCommand(keyRightRemoveCmd)
}
