package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmAbortCmd represents the "vm delete" command
var vmAbortCmd = &cobra.Command{
	Use:   "abort <vm-name>",
	Short: "Abort a VM creation",
	Long: `Abort a VM creation in the greenhouse (by its name).

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("DELETE", "/greenhouse/"+args[0], map[string]string{
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmAbortCmd)
	vmAbortCmd.Flags().StringP("revision", "r", "", "revision number")
}
