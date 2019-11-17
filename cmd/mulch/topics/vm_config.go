package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmConfigCmd represents the "vm config" command
var vmConfigCmd = &cobra.Command{
	Use:   "config <vm-name>",
	Short: "Get config of a VM",
	Long: `Return the config file used for VM creation.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("GET", "/vm/config/"+args[0], map[string]string{
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmConfigCmd)
	vmConfigCmd.Flags().StringP("revision", "r", "", "revision number")
}
