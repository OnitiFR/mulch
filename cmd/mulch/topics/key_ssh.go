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
a passphrase (except on Windows, see below), and load it into your SSH
agent ('ssh_pub_key' requires a running SSH agent):

ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519_mulch_api
ssh-add ~/.ssh/id_ed25519_mulch_api

('ssh-add' is not needed with GNOME, which loads ~/.ssh keys automatically)

Once this key is added, the key owner can replace the static 'key'
setting with 'ssh_pub_key' in their client configuration:

[[server]]
name = "my-mulch"
url = "https://…"
ssh_pub_key = "~/.ssh/id_ed25519_mulch_api.pub"

You can then disable the static key with 'key ssh only' (warning: test that
your SSH keys work before doing this).

-- Special cases:

Security keys: ed25519-sk / ecdsa-sk keys (ex: YubiKey) are supported, and
need a touch for each new session.

Remote host: without agent forwarding, you can use the agent of a desktop
session opened on that host, ex. with GNOME:

export SSH_AUTH_SOCK=$XDG_RUNTIME_DIR/gcr/ssh
ssh-add ~/.ssh/id_ed25519_mulch_api

Windows: mulch uses the Windows OpenSSH agent service. Enable it once,
in an administrator PowerShell:

Get-Service ssh-agent | Set-Service -StartupType Automatic
Start-Service ssh-agent

Then, as the key owner, load the key with the Windows 'ssh-add' (from
PowerShell, not from Git Bash). The key stays in the agent, even after
a reboot, and its passphrase is never asked again. Suggested workflow:
create the key without a passphrase, load it into the agent, then delete
the private key file (the agent keeps its own copy).
`,
}

func init() {
	keyCmd.AddCommand(keySSHCmd)
}
