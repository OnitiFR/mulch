package topics

import (
	"log"

	"github.com/spf13/cobra"
)

// backupUploadCmd represents the "backup upload" command
var backupUploadCmd = &cobra.Command{
	Use:   "upload <file.qcow2>",
	Short: "Upload a backup to server storage",
	// Long: ``,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/backup", map[string]string{})
		err := call.AddFile("file", args[0])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupUploadCmd)
	// backupUploadCmd.Flags().BoolP("force", "f", false, "overwrite existing file")
}
