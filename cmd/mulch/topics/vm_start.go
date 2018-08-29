package topics

import (
	"github.com/spf13/cobra"
)

// vmStartCmd represents the vmStart command
var vmStartCmd = &cobra.Command{
	Use:   "start <vm-name>",
	Short: "Start a VM",
	Long: `Start a VM by its name. The VM must be down to be started.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+ args[0], map[string]string{"action": "start"})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmStartCmd)
}
