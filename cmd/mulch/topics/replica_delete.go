package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// replicaDeleteCmd represents the "replica delete" command
var replicaDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a replica (database entry + raw disk file)",
	Long: `Delete a replica held by this peer.

This removes the replica database entry and its raw disk file. If the source
peer is still replicating this VM, the replica will reappear on the next sync.

Use --revision to target a specific revision. Without it, the only revision is
deleted, or the command fails if several revisions exist for that name.

See 'replica list' for replica names and revisions.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("DELETE", "/replica/"+args[0], map[string]string{
			"revision": revision,
		})
		call.Do()
	},
}

func init() {
	replicaCmd.AddCommand(replicaDeleteCmd)
	replicaDeleteCmd.Flags().StringP("revision", "r", "", "revision number")
}
