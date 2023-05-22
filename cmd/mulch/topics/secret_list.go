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

var secretListFlagBasic bool

// secretListCmd represents the "secret list" command
var secretListCmd = &cobra.Command{
	Use:   "list [path]",
	Short: "List all secrets",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		secretListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if keyListFlagBasic {
			client.GetExitMessage().Disable()
		}

		path := ""
		if len(args) > 0 {
			path = args[0]
		}

		call := client.GlobalAPI.NewCall("GET", "/secret", map[string]string{
			"path": path,
		})
		call.JSONCallback = secretListCB
		call.Do()
	},
}

func secretListCB(reader io.Reader, _ http.Header) {
	var data common.APISecretListEntries

	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if secretListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Key)
		}
	} else {
		strData := [][]string{}
		for _, line := range data {
			strData = append(strData, []string{
				line.Key,
				line.Modified.Format(time.RFC3339),
				line.AuthorKey,
			})
		}
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Secret", "Modified", "Author"})
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")
		table.AppendBulk(strData)
		table.Render()
	}
}

func init() {
	secretCmd.AddCommand(secretListCmd)
	secretListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
