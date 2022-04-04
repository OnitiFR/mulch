package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// backupExpireCmd represents the 'backup expire' command
var backupExpireCmd = &cobra.Command{
	Use:   "expire <disk-name> <expire-delay>",
	Short: "Change backup expiration",
	Long: `Change backup expiration (by its disk name)

See 'backup list' to get disk names.

Allowed units:	h, d, y (hours, days, years)
Allowed values: any positive integer

Give an empty string (or 0) to remove expiration.

Examples:
	mulch backup expire my-backup.qcow2 3h
	mulch backup expire my-backup.qcow2 0

See also -e argument on other commands:
	vm backup
	backup upload
`,
	Args:    cobra.ExactArgs(2),
	Aliases: []string{"remove"},
	Run: func(cmd *cobra.Command, args []string) {

		expireDuration, err := client.ParseExpiration(args[1])
		if err != nil {
			log.Fatalf("unable to parse expiration: %s", err)
		}

		call := client.GlobalAPI.NewCall("POST", "/backup/expire/"+args[0], map[string]string{
			"expire": client.DurationAsSecondsString(expireDuration),
		})
		call.Do()
	},
}

func init() {
	backupCmd.AddCommand(backupExpireCmd)
}
