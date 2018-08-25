package topics

import (
	"github.com/spf13/cobra"
)

// vmCreateCmd represents the vmCreate command
var vmCreateCmd = &cobra.Command{
	Use:   "create <config.toml>",
	Short: "Create a new VM",
	Long: `Create a new VM from a description file.

Example config:
xxx
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// fmt.Printf("vmCreate called with %s\n", args[0])
		call := globalAPI.NewCall("PUT", "/vm", map[string]string{})
		call.AddFile(args[0])
		call.Do()

	},
}

func init() {
	vmCmd.AddCommand(vmCreateCmd)
}
