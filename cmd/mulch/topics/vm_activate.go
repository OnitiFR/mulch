package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmActivateCmd represents the "vm activate" command
var vmActivateCmd = &cobra.Command{
	Use:   "activate <vm-name> <revision>",
	Short: "Activate a VM",
	Long: `Activate a specific VM revision, when multiple revision are
available.

The reverse proxy will instantly send requests to the new active revision,
and all VM commands (ex: lock, backup, ...) will defaults to this revision.

Revision "none" is equivalent to deactivate command.
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "activate",
			"revision": args[1],
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmActivateCmd)
}
