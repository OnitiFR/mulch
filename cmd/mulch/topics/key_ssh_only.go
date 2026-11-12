package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keySSHOnlyCmd represents the "key ssh only" command
var keySSHOnlyCmd = &cobra.Command{
	Use:   "only <key> on|off",
	Short: "Enable/disable SSH-only mode for the key",
	Long: `In SSH-only mode, the static API key and its generated SSH key pair
('sshpair') are disabled: only personal SSH keys can be used.

The key needs at least one personal SSH key (see 'key ssh add').
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		var enable string
		switch args[1] {
		case "on":
			enable = "true"
		case "off":
			enable = "false"
		default:
			log.Fatalf("invalid value '%s' (on/off)", args[1])
		}

		call := client.GlobalAPI.NewCall("POST", "/key/ssh-only/"+args[0], map[string]string{
			"enable": enable,
		})
		call.Do()
	},
}

func init() {
	keySSHCmd.AddCommand(keySSHOnlyCmd)
}
