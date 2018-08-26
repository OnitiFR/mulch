package topics

import (
	"log"

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
		call := globalAPI.NewCall("PUT", "/vm", map[string]string{})
		err := call.AddFile("config", args[0])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmCreateCmd)
}
