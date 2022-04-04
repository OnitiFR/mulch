package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// backupUploadCmd represents the "backup upload" command
var backupUploadCmd = &cobra.Command{
	Use:   "upload <file.qcow2>",
	Short: "Upload a backup to server storage",
	// Long: ``,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		expire, _ := cmd.Flags().GetString("expire")

		expireDuration, err := client.ParseExpiration(expire)
		if err != nil {
			log.Fatalf("unable to parse expiration: %s", err)
		}

		call := client.GlobalAPI.NewCall("POST", "/backup", map[string]string{
			"expire": client.DurationAsSecondsString(expireDuration),
		})
		err = call.AddFile("file", args[0])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupUploadCmd)
	backupUploadCmd.Flags().StringP("expire", "e", "", "expiration delay (ex: 2h, 10d, 1y)")
	// backupUploadCmd.Flags().BoolP("force", "f", false, "overwrite existing file")
}
