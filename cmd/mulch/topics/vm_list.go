package topics

import (
	"github.com/spf13/cobra"
)

// vmCreateCmd represents the vmCreate command
var vmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/vm", map[string]string{})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmListCmd)
}
