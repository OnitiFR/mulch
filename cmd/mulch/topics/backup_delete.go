package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// backupDeleteCmd represents the 'backup delete' command
var backupDeleteCmd = &cobra.Command{
	Use:   "delete <disk-name>",
	Short: "Delete a backup",
	Long: `Delete a backup (by its disk name)

See 'backup list' to get disk names.
`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"remove"},
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/backup/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupDeleteCmd)
}
