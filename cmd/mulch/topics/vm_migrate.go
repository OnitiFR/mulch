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
	Long: `Migrate a VM from this mulchd instance to another one ("destination").

Any pre-existing active VM will be deactivated in favor of the migrated VM.
Lock status will be preserved.

WARNING: --keep-source-active is for special cases:
- maintains service during migration
- source VM data may change AFTER migration backup, you may lose data
- won't work with a common frontal mulch-proxy (both VM can't be active at the same time)
`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		force, _ := cmd.Flags().GetBool("force")
		newRevision, _ := cmd.Flags().GetBool("new-revision")
		keepSourceActive, _ := cmd.Flags().GetBool("keep-source-active")

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":             "migrate",
			"destination":        args[1],
			"revision":           revision,
			"keep-source-active": strconv.FormatBool(keepSourceActive),
			"force":              strconv.FormatBool(force),
			"allow_new_revision": strconv.FormatBool(newRevision),
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmMigrateCmd)
	vmMigrateCmd.Flags().BoolP("force", "f", false, "force migration of a locked VM")
	vmMigrateCmd.Flags().BoolP("keep-source-active", "k", false, "keep source VM active during migration (WARNING)")
	vmMigrateCmd.Flags().BoolP("new-revision", "n", false, "allow a new revision with the same name")
	vmMigrateCmd.Flags().StringP("revision", "r", "", "revision number")
}
