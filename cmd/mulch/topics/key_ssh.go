package topics

import (
	"github.com/spf13/cobra"
)

// keySSHCmd represents the 'key ssh' command
var keySSHCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Personal SSH keys management",
	Long: `Manage personal SSH keys allowed to authenticate as an API key.

Once a personal SSH key is added, the key owner can replace the static
'key' setting with 'ssh_pub_key' in their client configuration:

[[server]]
name = "my-mulch"
url = "https://…"
ssh_pub_key = "~/.ssh/id_ed25519.pub"

Security keys (ed25519-sk / ecdsa-sk, ex: YubiKey) are supported, and
need a touch for each new session.

You can then disable the static key with 'key ssh only' (warning: test that
your SSH keys work before doing this)
`,
}

func init() {
	keyCmd.AddCommand(keySSHCmd)
}
