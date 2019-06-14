package topics

import (
	"github.com/spf13/cobra"
)

// vmDeleteCmd represents the "vm delete" command
var vmDeleteCmd = &cobra.Command{
	Use:   "delete <vm-name>",
	Short: "Delete a VM",
	Long: `Delete a VM (by its name) and all related volumes and disks.
I REPEAT: no data remains after this operation!
Note: A locked VM can't be deleted.

See 'vm list' for VM Names.
`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"remove"},
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := globalAPI.NewCall("DELETE", "/vm/"+args[0], map[string]string{
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmDeleteCmd)
	vmDeleteCmd.Flags().StringP("revision", "r", "", "revision number")
}
