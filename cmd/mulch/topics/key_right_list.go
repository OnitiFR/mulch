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

// keyRightListCmd represents the "key right list" command
var keyRightListCmd = &cobra.Command{
	Use:   "list <key>",
	Short: "List key rights",
	// Long: ``,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/key/right/"+args[0], map[string]string{})
		call.JSONCallback = keyRightListCB
		call.Do()
	},
}

func keyRightListCB(reader io.Reader, headers http.Header) {
	var data common.APIKeyRightEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "No limited rights, everything is allowed for this key.\n")
		return
	}

	for _, line := range data {
		fmt.Printf("%s\n", line)
	}

}

func init() {
	keyRightCmd.AddCommand(keyRightListCmd)
}
