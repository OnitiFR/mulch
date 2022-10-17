package topics

import (
	"github.com/spf13/cobra"
)

// secretCmd represents the "secret" command
var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Secrets management",
}

func init() {
	rootCmd.AddCommand(secretCmd)
}
