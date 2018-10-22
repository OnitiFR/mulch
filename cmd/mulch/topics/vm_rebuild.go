package topics

import (
	"github.com/spf13/cobra"
)

// vmRebuildCmd represents the "vm rebuild" command
var vmRebuildCmd = &cobra.Command{
	Use:   "rebuild <vm-name>",
	Short: "Rebuild a VM",
	Long: `Backup a VM, DELETE IT and re-create it from the backup.

Currently, this process is NOT a transaction: if new VM creation
fails, your old VM is lost anyway! Temporay backup is only deleted
if the rebuild is successful, though.

Anyway, you should consider this operation as a dangerous one, since
the result relies on backup/restore scripts correctness. You may lose
data in the process if one of those scripts "forgets" some data.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{"action": "rebuild"})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRebuildCmd)
}
