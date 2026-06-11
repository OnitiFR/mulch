package topics

import (
	"strconv"

	"github.com/spf13/cobra"
)

// replicaID rebuilds a VM ID (name + optional revision) the same way the
// server's VMName.ID() does, for display and for passing to "replica delete".
func replicaID(name string, revision int) string {
	if revision == 0 {
		return name
	}
	return name + "-r" + strconv.Itoa(revision)
}

// replicaCmd represents the "replica" command
var replicaCmd = &cobra.Command{
	Use:   "replica",
	Short: "Replica management (received from peers)",
	Long: `Manage replicas held by this mulchd: VM disks replicated to us by a peer,
ready to be promoted to local VMs in case of failover.
`,
}

func init() {
	rootCmd.AddCommand(replicaCmd)
}
