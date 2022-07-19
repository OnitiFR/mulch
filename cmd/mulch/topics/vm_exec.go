package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmExecCmd represents the "vm exec" command
var vmExecCmd = &cobra.Command{
	Use:   "exec <vm-name> <user> <script-file>",
	Short: "Execute a script in VM",
	Long: `Execture a shell script inside the VM as the specified user.

This command is particularly useful when testing new prepare, install,
backup and restore scripts.

Example:
  mulch vm exec myvm admin script.sh
`,
	Args: cobra.ExactArgs(3),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action": "exec",
			"as":     args[1],
		})
		err := call.AddFile("script", args[2])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmExecCmd)
}
