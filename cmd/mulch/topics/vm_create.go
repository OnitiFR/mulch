package topics

import (
	"log"

	"github.com/spf13/cobra"
)

// vmCreateCmd represents the "vm create" command
var vmCreateCmd = &cobra.Command{
	Use:   "create <config.toml>",
	Short: "Create a new VM",
	Long: `Create a new VM from a description file.

See sample-vm.toml for an example, or get config
from an existing VM using [unimplemented yet]
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		restore, _ := cmd.Flags().GetString("restore")

		call := globalAPI.NewCall("POST", "/vm", map[string]string{
			"restore": restore,
		})
		err := call.AddFile("config", args[0])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmCreateCmd)
	vmCreateCmd.Flags().StringP("restore", "r", "", "backup to restore")
}
