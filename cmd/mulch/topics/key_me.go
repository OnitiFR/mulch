package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyMeCmd represents the "key list" command
var keyMeCmd = &cobra.Command{
	Use:   "me",
	Short: "Show my API key comment",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		call := client.GlobalAPI.NewCall("GET", "/key/me/comment", map[string]string{})
		call.Do()
	},
}

func init() {
	keyCmd.AddCommand(keyMeCmd)
}
