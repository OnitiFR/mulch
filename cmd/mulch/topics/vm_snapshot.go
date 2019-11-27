package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// TODO: add revision support?

// vmSnapshotCmd represents the "vm snapshot" command
var vmSnapshotCmd = &cobra.Command{
	Use:   "snapshot <vm-name>",
	Short: "Capture a new VM snapshot",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action": "snapshot",
		})
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmSnapshotCmd)
}
