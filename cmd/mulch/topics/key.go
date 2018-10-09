package topics

import (
	"github.com/spf13/cobra"
)

// keyCmd represents the key command
var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "API keys management",
	Long:  `Manage API keys.`,
}

func init() {
	rootCmd.AddCommand(keyCmd)
}
