package topics

import (
	"github.com/spf13/cobra"
)

// vmCmd represents the vm command
var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Virtual Machines management",
}

func init() {
	rootCmd.AddCommand(vmCmd)
}
