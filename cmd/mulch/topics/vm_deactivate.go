package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmDeactivateCmd represents the "vm deactivate" command
var vmDeactivateCmd = &cobra.Command{
	Use:   "deactivate <vm-name>",
	Short: "Deactivate a VM",
	Long: `Deactivate a VM, so the reverse proxy will instantly stop sending requests to this VM.

You'll then need to use --revision flag for many other
commands (start, stop, delete, etc). See activate command too.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "activate",
			"force":    strconv.FormatBool(force),
			"revision": "none",
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmDeactivateCmd)
	vmDeactivateCmd.Flags().BoolP("force", "f", false, "force deactivation of a locked VM")
}
