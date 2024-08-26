package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// trustListCmd represents the "trust list" command
var trustListCmd = &cobra.Command{
	Use:   "list [vm]",
	Short: "List your forwarded SSH keys",
	Long: `List your forwarded SSH keys.

You can get the SHA256 fingerprint of a key with the command:
  ssh-keygen -lf ~/.ssh/key.pub

Using the follwing command in the corresponding VM may also be useful:
  ssh-add -l

Warning: Remember that all forwarded keys can be used by other users on the VM when you are connected to it.
`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		vm := ""
		if len(args) == 1 {
			vm = args[0]
		}
		call := client.GlobalAPI.NewCall("GET", "/key/trust/list/"+vm, map[string]string{})
		call.JSONCallback = TrustListCB
		call.Do()
	},
}

func TrustListCB(reader io.Reader, _ http.Header) {
	var data common.APITrustListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("No keys forwarded to any VM.\n")
		return
	}

	strData := [][]string{}

	for _, line := range data {
		strData = append(strData, []string{
			line.VM,
			line.Fingerprint,
			line.AddedAt.Format(time.RFC3339),
		})
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"VM", "Fingerprint", "Added at"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.SetAutoWrapText(true)
	table.AppendBulk(strData)
	table.Render()
}

func init() {
	trustCmd.AddCommand(trustListCmd)
}
