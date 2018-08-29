package topics

import (
	"github.com/spf13/cobra"
)

// vmStopCmd represents the vmStop command
var vmStopCmd = &cobra.Command{
	Use:   "stop <vm-name>",
	Short: "Stop a VM",
	Long: `Stop a VM by its name. The VM must be up to be stopped.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+ args[0], map[string]string{"action": "stop"})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmStopCmd)
}
