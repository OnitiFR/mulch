package topics

import (
	"github.com/spf13/cobra"
)

// peerCmd represents the "peer" command
var peerCmd = &cobra.Command{
	Use:   "peer",
	Short: "Show peers",
	Long: `Shows a list of vm migration possible destinations.
`,
}

func init() {
	rootCmd.AddCommand(peerCmd)
}
