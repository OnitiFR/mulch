package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var replicationSyncNowCmd = &cobra.Command{
	Use:   "sync-now <vm-name>",
	Short: "Trigger an immediate replication sync",
	Long: `Trigger an immediate replication sync for a VM, without waiting for the
next scheduled interval. This also bypasses any active backoff.

Useful when:
  - the replication interval is long and you want to sync before a risky operation
  - replication is in backoff/pause after errors that have been fixed
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("POST", "/replication/"+args[0], map[string]string{
			"action":   "sync-now",
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	replicationCmd.AddCommand(replicationSyncNowCmd)
	replicationSyncNowCmd.Flags().StringP("revision", "r", "", "revision number")
}
