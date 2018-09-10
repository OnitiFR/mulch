package topics

import (
	"github.com/spf13/cobra"
)

// vmConfigCmd represents the vmConfig command
var vmConfigCmd = &cobra.Command{
	Use:   "config <vm-name>",
	Short: "Get config of a VM",
	Long: `Return the config file used for VM creation.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/vm/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmConfigCmd)
}
