package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var seedListFlagBasic bool

// seedListCmd represents the "seed list" command
var seedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Seeds",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		seedListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if seedListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/seed", map[string]string{})
		call.JSONCallback = seedsCB
		call.Do()
	},
}

func seedsCB(reader io.Reader, _ http.Header) {
	var data common.APISeedListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("Currently, no seed exists. You must declare seeds in your Mulch server.\n")
		return
	}

	if seedListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Name)
		}
	} else {
		strData := [][]string{}
		red := color.New(color.FgHiRed).SprintFunc()
		green := color.New(color.FgHiGreen).SprintFunc()
		for _, line := range data {
			state := red("not-ready")
			if line.Ready {
				state = green("ready")
			}

			strData = append(strData, []string{
				line.Name,
				state,
				line.LastModified.Format(time.RFC3339),
				(datasize.ByteSize(line.Size) * datasize.B).HR(),
			})
		}

		headers := []string{"Name", "Ready", "Last Modified", "Size"}
		client.RenderTable(headers, strData)
	}
}

func init() {
	seedCmd.AddCommand(seedListCmd)
	seedListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
