package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmMigrateCmd represents the "vm migrate" command
var vmMigrateCmd = &cobra.Command{
	Use:   "migrate <vm-name> <destination-peer>",
	Short: "Migrate a VM",
	Long:  `Migrate a from this mulch server to another one ("destination").`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		force, _ := cmd.Flags().GetBool("force")
		newRevision, _ := cmd.Flags().GetBool("new-revision")

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":             "migrate",
			"destination":        args[1],
			"revision":           revision,
			"force":              strconv.FormatBool(force),
			"allow_new_revision": strconv.FormatBool(newRevision),
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmMigrateCmd)
	vmMigrateCmd.Flags().BoolP("force", "f", false, "force migration of a locked VM")
	vmMigrateCmd.Flags().BoolP("new-revision", "n", false, "allow a new revision with the same name")
	vmMigrateCmd.Flags().StringP("revision", "r", "", "revision number")
}
