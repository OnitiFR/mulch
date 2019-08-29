package topics

import (
	"log"
	"strconv"

	"github.com/spf13/cobra"
)

// vmCreateCmd represents the "vm create" command
var vmCreateCmd = &cobra.Command{
	Use:   "create <config.toml>",
	Short: "Create a new VM",
	Long: `Create a new VM from a description file.

See sample-vm.toml for an example, or get config
from an existing VM using [unimplemented yet]
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		restore, _ := cmd.Flags().GetString("restore")
		newRevision, _ := cmd.Flags().GetBool("new-revision")
		inactive, _ := cmd.Flags().GetBool("inactive")
		keepOnFailure, _ := cmd.Flags().GetBool("keep-on-failure")

		call := globalAPI.NewCall("POST", "/vm", map[string]string{
			"restore":            restore,
			"allow_new_revision": strconv.FormatBool(newRevision),
			"inactive":           strconv.FormatBool(inactive),
			"keep_on_failure":    strconv.FormatBool(keepOnFailure),
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
	vmCreateCmd.Flags().StringP("restore", "r", "", "backup to restore")
	vmCreateCmd.MarkFlagCustom("restore", "__internal_list_backups")
	vmCreateCmd.Flags().BoolP("new-revision", "n", false, "allow a new revision with the same name")
	vmCreateCmd.Flags().BoolP("inactive", "i", false, "do not set this instance as active")
	vmCreateCmd.Flags().BoolP("keep-on-failure", "k", false, "keep VM on script failure (useful for debug)")
}
