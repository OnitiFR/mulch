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
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var keyListFlagBasic bool

// keyListCmd represents the "key list" command
var keyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List keys",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		keyListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if keyListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/key", map[string]string{})
		call.JSONCallback = keyListCB
		call.Do()
	},
}

func keyListCB(reader io.Reader, _ http.Header) {
	var data common.APIKeyListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if keyListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Comment)
		}
	} else {
		if len(data) == 0 {
			fmt.Printf("No result. But you've called the API. WTF.\n")
			return
		}

		strData := [][]string{}
		for _, line := range data {
			strData = append(strData, []string{
				line.Comment,
				fmt.Sprintf("%d", line.RightCount),
				fmt.Sprintf("%d", line.FingerprintCount),
			})
		}
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Comment", "Rights", "Fingerprints"})
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")
		table.AppendBulk(strData)
		table.Render()
	}
}

func init() {
	keyCmd.AddCommand(keyListCmd)
	keyListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
