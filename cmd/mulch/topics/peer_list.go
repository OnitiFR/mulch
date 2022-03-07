package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// peerListCmd represents the "peer list" command
var peerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List peers",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/peer", map[string]string{})
		call.Do()
	},
}

func init() {
	peerCmd.AddCommand(peerListCmd)
}
