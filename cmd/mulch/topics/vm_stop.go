package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmStopCmd represents the "vm stop" command
var vmStopCmd = &cobra.Command{
	Use:   "stop <vm-name>",
	Short: "Stop a VM",
	Long: `Stop a VM by its name. The VM must be up to be stopped.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		emergency, _ := cmd.Flags().GetBool("emergency")
		force, _ := cmd.Flags().GetBool("force")
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":    "stop",
			"emergency": strconv.FormatBool(emergency),
			"force":     strconv.FormatBool(force),
			"revision":  revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmStopCmd)
	vmStopCmd.Flags().StringP("revision", "r", "", "revision number")
	vmStopCmd.Flags().BoolP("emergency", "e", false, "allow emergency stop (may corrupt data)")
	vmStopCmd.Flags().BoolP("force", "f", false, "force stop a locked VM")
}
