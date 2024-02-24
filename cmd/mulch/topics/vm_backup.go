package topics

import (
	"log"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
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
		noCompress, _ := cmd.Flags().GetBool("no-compress")
		expire, _ := cmd.Flags().GetString("expire")

		expireDuration, err := client.ParseDuration(expire)
		if err != nil {
			log.Fatalf("unable to parse expiration: %s", err)
		}

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":         "backup",
			"revision":       revision,
			"allow-compress": strconv.FormatBool(!noCompress),
			"expire":         client.DurationAsSecondsString(expireDuration),
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmBackupCmd)
	vmBackupCmd.Flags().StringP("revision", "r", "", "revision number")
	vmBackupCmd.Flags().BoolP("no-compress", "n", false, "disable compression (faster but bigger backup)")
	vmBackupCmd.Flags().StringP("expire", "e", "", "expiration delay (ex: 2h, 10d, 1y)")
}
