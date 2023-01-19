package topics

import (
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
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "restart",
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRestartCmd)
	vmRestartCmd.Flags().StringP("revision", "r", "", "revision number")
}
