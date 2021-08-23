package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyRightAddCmd represents the "key right add" command
var keyRightAddCmd = &cobra.Command{
	Use:   "add <key> <right>",
	Short: "Add right to the key",
	Long: `Add a right to the key

The right must follow this format:
METHOD path header1=value1 header2=value2

You can use "*" joker in method, path, and header values.

WARNING: no rights means full privileges.

-- Examples:
Allow to list VMs:
GET /vm

Allow VM backup:
POST /vm/* action=backup

Allow to run a specific action to a specific VM:
POST /vm/myvm action=do do_action=logs

Allow SSH access:
GET /sshpair
`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/key/right/"+args[0], map[string]string{
			"right": args[1],
		})
		call.Do()
	},
}

func init() {
	keyRightCmd.AddCommand(keyRightAddCmd)
}
