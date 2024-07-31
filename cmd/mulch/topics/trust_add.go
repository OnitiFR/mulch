package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// trustAddCmd represents the "trust add" command
var trustAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a trusted VM to my key",
	Long: `Add a trusted VM to my key.

WARNING: trusted VMs have access to your SSH agent, anyone with access to
this VM will be able to use all your SSH keys while you are connected!
`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/key/trust/list/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustAddCmd)
}
