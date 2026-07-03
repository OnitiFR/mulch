package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// replicaPromoteCmd represents the "replica promote" command
var replicaPromoteCmd = &cobra.Command{
	Use:   "promote <name>",
	Short: "Promote a replica to a new local VM (failover)",
	Long: `Promote a replica held by this peer to a new local VM.

This is the failover action, when the source peer is down: the replicated
disk is booted as-is in a new local VM.

Incoming replication for this VM name is durably refused, before anything
else: even if the source peer comes back, it can't overwrite the promoted
disk. The replica entry is kept as a "promoted" tombstone for that purpose;
'replica delete' removes it explicitly (allowing replication again).

Use --revision to target a specific revision. Without it, the only revision
is promoted, or the command fails if several revisions exist for that name.

See 'replica list' for replica names and revisions.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("POST", "/replica/"+args[0], map[string]string{
			"action":   "promote",
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	replicaCmd.AddCommand(replicaPromoteCmd)
	replicaPromoteCmd.Flags().StringP("revision", "r", "", "revision number")
}
