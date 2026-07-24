package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// replicaPromoteCmd represents the "replica promote" command
var replicaPromoteCmd = &cobra.Command{
	Use:   "promote <name>",
	Short: "Promote a replica to a new local VM (failover)",
	Long: `Promote a replica held by this peer to a new local VM.

DANGEROUS failover — use only when the source peer is really down. It does
NOT verify that the source is unreachable, and its effects are hard to undo,
so it refuses to run without --force.

What it does:
  - boots the replicated disk as-is: up to one replication_interval of data
    written on the source since the last sync is lost;
  - when the source peer comes back, forces it to STAND DOWN — its VM is
    deactivated and its domains are pinned to this peer;
  - durably refuses incoming replication for this VM name (done first, so a
    returning source can't overwrite the promoted disk). The replica is kept
    as a "promoted" tombstone; 'replica delete' clears it and re-allows
    replication.

Beware split-brain: a down mulchd does NOT mean the source is down. Its guest
VM may still be running and the parent proxy may still route live traffic to
it, so the source keeps serving users and writing to a disk that is no longer
replicated. Promoting then leaves two diverging copies, and the later
stand-down destroys everything the source served since the last sync.

Use --revision to target a specific revision. Without it, the sole revision is
promoted, or the command fails if several exist. See 'replica list' for names
and revisions.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		force, _ := cmd.Flags().GetBool("force")
		call := client.GlobalAPI.NewCall("POST", "/replica/"+args[0], map[string]string{
			"action":   "promote",
			"revision": revision,
			"force":    strconv.FormatBool(force),
		})
		call.Do()
	},
}

func init() {
	replicaCmd.AddCommand(replicaPromoteCmd)
	replicaPromoteCmd.Flags().StringP("revision", "r", "", "revision number")
	replicaPromoteCmd.Flags().BoolP("force", "f", false, "confirm this dangerous failover (required)")
}
