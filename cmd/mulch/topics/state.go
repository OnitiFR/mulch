package topics

import (
	"github.com/spf13/cobra"
)

// stateCmd represents the "state" command
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage state of all VMs",
}

func init() {
	rootCmd.AddCommand(stateCmd)
}
