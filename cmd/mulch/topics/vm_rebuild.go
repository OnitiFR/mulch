package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmRebuildCmd represents the "vm rebuild" command
var vmRebuildCmd = &cobra.Command{
	Use:   "rebuild <vm-name>",
	Short: "Rebuild a VM",
	Long: `Recreate a VM using its own backup.

Warning: you should consider this operation as a dangerous one, since
the result relies on backup/restore scripts correctness. You may lose
data in the process if one of those scripts "forgets" some data.

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		lock, _ := cmd.Flags().GetBool("lock")
		force, _ := cmd.Flags().GetBool("force")
		revision, _ := cmd.Flags().GetString("revision")

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "rebuild",
			"lock":     strconv.FormatBool(lock),
			"force":    strconv.FormatBool(force),
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRebuildCmd)
	vmRebuildCmd.Flags().BoolP("force", "f", false, "force rebuild of a locked VM")
	vmRebuildCmd.Flags().BoolP("lock", "l", false, "lock VM on rebuild success")
	vmRebuildCmd.Flags().StringP("revision", "r", "", "revision number")
}
