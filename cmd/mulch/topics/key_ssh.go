package topics

import (
	"github.com/spf13/cobra"
)

// keySSHCmd represents the 'key ssh' command
var keySSHCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Personal SSH keys management",
	Long: `Manage personal SSH keys allowed to authenticate as an API key.

Use a dedicated SSH key for mulch (not your everyday key), protected by
a passphrase:

ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519_mulch_api

Once this key is added, the key owner can replace the static 'key'
setting with 'ssh_pub_key' in their client configuration:

[[server]]
name = "my-mulch"
url = "https://…"
ssh_pub_key = "~/.ssh/id_ed25519_mulch_api.pub"

Security keys (ed25519-sk / ecdsa-sk, ex: YubiKey) are supported, and
need a touch for each new session.

'ssh_pub_key' requires a running SSH agent. Without agent forwarding,
you can use the agent of a desktop session opened on that host, ex. with GNOME:

export SSH_AUTH_SOCK=$XDG_RUNTIME_DIR/gcr/ssh
ssh-add ~/.ssh/id_ed25519_mulch_api

You can then disable the static key with 'key ssh only' (warning: test that
your SSH keys work before doing this)
`,
}

func init() {
	keyCmd.AddCommand(keySSHCmd)
}
