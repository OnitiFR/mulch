package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keySSHRemoveCmd represents the "key ssh remove" command
var keySSHRemoveCmd = &cobra.Command{
	Use:   "remove <key> <fingerprint>",
	Short: "Remove a personal SSH key from an API key",
	Long: `Remove a personal SSH key from an API key, using its fingerprint
(see 'key ssh list'). API sessions opened with this SSH key are revoked
(but open SSH connections are not closed).
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("DELETE", "/key/ssh/"+args[0], map[string]string{
			"fingerprint": args[1],
		})
		call.Do()
	},
}

func init() {
	keySSHCmd.AddCommand(keySSHRemoveCmd)
}
