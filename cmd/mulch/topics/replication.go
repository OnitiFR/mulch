package topics

import (
	"github.com/spf13/cobra"
)

// replicationCmd represents the "replication" command
var replicationCmd = &cobra.Command{
	Use:     "replication",
	Short:   "Disk replication management",
	Aliases: []string{"repl"},
	Long: `Manage VM disk replication towards a peer mulchd.
`,
}

func init() {
	rootCmd.AddCommand(replicationCmd)
}
