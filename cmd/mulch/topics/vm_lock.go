package topics

import (
	"github.com/spf13/cobra"
)

// vmLockCmd represents the "vm lock" command
var vmLockCmd = &cobra.Command{
	Use:   "lock <vm-name>",
	Short: "Lock a VM",
	Long: `Lock a VM (by its name). It's not possible to delete a locked VM.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{"action": "lock"})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmLockCmd)
}
