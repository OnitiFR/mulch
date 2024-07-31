package topics

import (
	"github.com/spf13/cobra"
)

// trustCmd represents the "trust" command
var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trusted VMs configuration",
}

func init() {
	rootCmd.AddCommand(trustCmd)
}
