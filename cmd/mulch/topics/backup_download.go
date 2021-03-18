package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// backupDownloadCmd represents the "backup download" command
var backupDownloadCmd = &cobra.Command{
	Use:   "download <disk-name>",
	Short: "Download a backup to client disk",
	// Long: ``,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backupName := args[0]

		force, _ := cmd.Flags().GetBool("force")

		if common.PathExist(backupName) && !force {
			log.Fatalf("file %s already exists (use -f for overwrite)", backupName)
		}

		call := client.GlobalAPI.NewCall("GET", "/backup/"+backupName, map[string]string{})
		call.DestFilePath = backupName
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupDownloadCmd)
	backupDownloadCmd.Flags().BoolP("force", "f", false, "overwrite existing file")
}
