package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmRestartCmd represents the "vm restart" command
var vmRestartCmd = &cobra.Command{
	Use:   "restart <vm-name>",
	Short: "Restart a VM",
	Long: `Restart a VM by its name. The VM must be up to be restarted.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		emergency, _ := cmd.Flags().GetBool("emergency")
		force, _ := cmd.Flags().GetBool("force")
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":    "restart",
			"emergency": strconv.FormatBool(emergency),
			"force":     strconv.FormatBool(force),
			"revision":  revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRestartCmd)
	vmRestartCmd.Flags().StringP("revision", "r", "", "revision number")
	vmRestartCmd.Flags().BoolP("emergency", "e", false, "allow emergency stop (may corrupt data)")
	vmRestartCmd.Flags().BoolP("force", "f", false, "force restart a locked VM")
}
