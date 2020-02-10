package topics

import (
	"log"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmCreateCmd represents the "vm create" command
var vmCreateCmd = &cobra.Command{
	Use:   "create <config.toml>",
	Short: "Create a new VM",
	Long: `Create a new VM from a description file.

See sample-vm.toml for an example, or get config from an existing
VM using 'vm config'.

You can restore data in this new VM from an existing backup (-r) or
from another VM (-R).
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		restore, _ := cmd.Flags().GetString("restore")
		restoreVM, _ := cmd.Flags().GetString("restore-vm")
		newRevision, _ := cmd.Flags().GetBool("new-revision")
		inactive, _ := cmd.Flags().GetBool("inactive")
		keepOnFailure, _ := cmd.Flags().GetBool("keep-on-failure")
		lock, _ := cmd.Flags().GetBool("lock")

		call := client.GlobalAPI.NewCall("POST", "/vm", map[string]string{
			"restore":            restore,
			"restore-vm":         restoreVM,
			"allow_new_revision": strconv.FormatBool(newRevision),
			"inactive":           strconv.FormatBool(inactive),
			"keep_on_failure":    strconv.FormatBool(keepOnFailure),
			"lock":               strconv.FormatBool(lock),
		})
		err := call.AddFile("config", args[0])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmCreateCmd)

	vmCreateCmd.Flags().StringP("restore", "r", "", "restore from a backup")
	vmCreateCmd.MarkFlagCustom("restore", "__internal_list_backups")

	vmCreateCmd.Flags().StringP("restore-vm", "R", "", "restore from a running VM")
	vmCreateCmd.MarkFlagCustom("restore-vm", "__internal_list_vms")

	vmCreateCmd.Flags().BoolP("new-revision", "n", false, "allow a new revision with the same name")
	vmCreateCmd.Flags().BoolP("inactive", "i", false, "do not set this instance as active")
	vmCreateCmd.Flags().BoolP("keep-on-failure", "k", false, "keep VM on script failure (useful for debug)")
	vmCreateCmd.Flags().BoolP("lock", "l", false, "lock VM after creation")
}
