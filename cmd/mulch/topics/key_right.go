package topics

import (
	"github.com/spf13/cobra"
)

// keyRightCmd represents the 'key right' command
var keyRightCmd = &cobra.Command{
	Use:   "right",
	Short: "API key rights management",
	Long:  `Manage API key rights`,
}

func init() {
	keyCmd.AddCommand(keyRightCmd)
}
