package topics

import (
	"github.com/spf13/cobra"
)

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backups management",
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
