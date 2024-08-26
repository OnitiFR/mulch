package topics

import (
	"log"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

// trustForwardCmd represents the "trust forward" command
var trustForwardCmd = &cobra.Command{
	Use:   "forward <vm> <ssh-pub-file>",
	Short: "Forward a SSH key to a VM",

	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		// read the public key file and get it's SHA256
		pubKeyFile := args[1]
		pubKeyData, err := os.ReadFile(pubKeyFile)
		if err != nil {
			log.Fatalf("error reading public key file: %s", err)
		}

		pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubKeyData)
		if err != nil {
			log.Fatalf("error parsing public key: %s", err)
		}

		sha256 := ssh.FingerprintSHA256(pubKey)

		call := client.GlobalAPI.NewCall("POST", "/key/trust/list/"+args[0], map[string]string{
			"fingerprint": sha256,
		})
		call.Do()
	},
}

func init() {
	trustCmd.AddCommand(trustForwardCmd)
}
