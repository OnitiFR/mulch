package topics

import (
	"github.com/spf13/cobra"
)

// vmUnlockCmd represents the "vm unlock" command
var vmUnlockCmd = &cobra.Command{
	Use:   "unlock <vm-name>",
	Short: "Unlock a VM",
	Long: `Unlock a VM (by its name), allowing the VM to be deleted.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{"action": "unlock"})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmUnlockCmd)
}
