package topics

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keySSHAddCmd represents the "key ssh add" command
var keySSHAddCmd = &cobra.Command{
	Use:   "add <key> <public-key-file>",
	Short: "Add a personal SSH key to an API key",
	Long: `Add a personal SSH public key to an API key (use "-" to read from stdin).

Accepted types: ed25519, ecdsa, RSA (2048 bits or more) and their
security key variants (ed25519-sk, ecdsa-sk).

Examples:
  mulch key ssh add alice ~/.ssh/id_ed25519_mulch_api.pub
  ssh-add -L | grep alice@laptop | mulch key ssh add alice -
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		var data []byte
		var err error

		if args[1] == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			if !strings.HasSuffix(args[1], ".pub") {
				log.Fatalf("'%s' does not look like a public key file (*.pub)", args[1])
			}
			data, err = os.ReadFile(args[1])
		}
		if err != nil {
			log.Fatal(err.Error())
		}

		call := client.GlobalAPI.NewCall("POST", "/key/ssh/"+args[0], map[string]string{
			"pubkey": strings.TrimSpace(string(data)),
		})
		call.Do() // exits on failure

		keySSHAddPrintHint(args[1])
	},
}

// keySSHAddPrintHint shows the client configuration line to use this key
func keySSHAddPrintHint(file string) {
	if file == "-" {
		return
	}

	// absolute path, with ~ for the home directory
	value, err := filepath.Abs(file)
	if err != nil {
		return
	}
	if rel, err := filepath.Rel(client.GlobalHome, value); err == nil && !strings.HasPrefix(rel, "..") {
		value = "~/" + rel
	}

	fmt.Printf(`
To authenticate with this SSH key, replace 'key' with this line in the
[[server]] section of '%s' (in %s):

ssh_pub_key = %q
`, client.GlobalConfig.Server.Name, client.GlobalConfig.ConfigFile, value)
}

func init() {
	keySSHCmd.AddCommand(keySSHAddCmd)
}
