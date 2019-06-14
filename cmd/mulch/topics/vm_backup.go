package topics

import (
	"github.com/spf13/cobra"
)

// vmBackupCmd represents the "vm backup" command
var vmBackupCmd = &cobra.Command{
	Use:   "backup <vm-name>",
	Short: "backup a VM",
	Long: `Backup a VM (by its name).

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "backup",
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmBackupCmd)
	vmBackupCmd.Flags().StringP("revision", "r", "", "revision number")
}
