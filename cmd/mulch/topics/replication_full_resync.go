package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var replicationFullResyncCmd = &cobra.Command{
	Use:   "full-resync <vm-name>",
	Short: "Force and trigger an immediate full copy",
	Long: `Drop the current checkpoint state and trigger an immediate full copy of the
whole disk, instead of an incremental sync. Any active backoff is bypassed.

Useful when:
  - incremental syncs fail with "checkpoint inconsistent / bitmap missing or
    broken" (typically after the VM was stopped uncleanly or the host crashed,
    which leaves the qcow2 dirty bitmap in an inconsistent state)
  - you want to start the replication chain from a clean state
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("POST", "/replication/"+args[0], map[string]string{
			"action":   "full-resync",
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	replicationCmd.AddCommand(replicationFullResyncCmd)
	replicationFullResyncCmd.Flags().StringP("revision", "r", "", "revision number")
}
