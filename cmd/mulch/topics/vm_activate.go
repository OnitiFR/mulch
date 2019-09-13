package topics

import (
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
`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "activate",
			"revision": args[1],
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmActivateCmd)
}
