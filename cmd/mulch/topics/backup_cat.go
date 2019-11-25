package topics

import (
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// backupCatCmd represents the "backup cat" command
var backupCatCmd = &cobra.Command{
	Use:   "cat <disk-name>",
	Short: "Download a backup to stdout",
	Long: `Download a backup to stdout, allowing you to pipe output somewhere.

Errors are still sent to stderr.
	`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backupName := args[0]

		call := client.GlobalAPI.NewCall("GET", "/backup/"+backupName, map[string]string{})
		call.DestStream = os.Stdout
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupCatCmd)
}
