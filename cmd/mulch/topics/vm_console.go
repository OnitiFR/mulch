package topics

import (
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmConsoleCmd represents the "vm console" command
var vmConsoleCmd = &cobra.Command{
	Use:   "console <vm-name>",
	Short: "Flush VM console",
	Long: `Show and flush VM main console/terminal.

Useful to see the boot process, debug a VM, output specialized streams, etc.

A rolling buffer stores latest console output. This command will flush
this buffer to your standard output. Flushed content is lost.

Warning: only one client should read the console at a time.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("GET", "/vm/console/"+args[0], map[string]string{
			"revision": revision,
		})
		call.DestStream = os.Stdout
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmConsoleCmd)
	vmConsoleCmd.Flags().StringP("revision", "r", "", "revision number")
}
