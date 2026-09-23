package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyRightClearCmd represents the "key right clear" command
var keyRightClearCmd = &cobra.Command{
	Use:   "clear <key>",
	Short: "Remove all rights from the key (full privileges)",
	Long: `Remove all rights from the key

WARNING: no rights means full privileges.
`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/key/right/"+args[0], map[string]string{
			"all": "true",
		})
		call.Do()
	},
}

func init() {
	keyRightCmd.AddCommand(keyRightClearCmd)
}
