package topics

import (
	"github.com/spf13/cobra"
)

// vmDeleteCmd represents the vmDelete command
var vmDeleteCmd = &cobra.Command{
	Use:   "delete <vm-name>",
	Short: "Delete a VM",
	Long: `Delete a VM (by its name) and all related volumes and disks.
I REPEAT: no data remains after this operation!
Note: A locked VM can't be deleted.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("DELETE", "/vm/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmDeleteCmd)
}
