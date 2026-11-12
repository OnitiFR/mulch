package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// keySSHListCmd represents the "key ssh list" command
var keySSHListCmd = &cobra.Command{
	Use:   "list <key>",
	Short: "List personal SSH keys of the key",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/key/ssh/"+args[0], map[string]string{})
		call.JSONCallback = keySSHListCB
		call.Do()
	},
}

func keySSHListCB(reader io.Reader, _ http.Header) {
	var data common.APIKeySSHEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "No personal SSH key for this API key.\n")
		return
	}

	// one line per key, like 'ssh-add -l' (full fingerprint, easy to copy)
	for _, line := range data {
		fmt.Printf("%s %s %s (added %s)\n",
			line.Fingerprint,
			line.Type,
			line.Comment,
			line.AddedAt.Format("2006-01-02 15:04"),
		)
	}
}

func init() {
	keySSHCmd.AddCommand(keySSHListCmd)
}
