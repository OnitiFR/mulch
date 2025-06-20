package topics

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var secretListVerbose bool
var secretListOrphanOny bool
var secretListSortByTotal bool

// secretUsageCmd represents the "secret list" command
var secretUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "List secrets usage",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/secret-usage", map[string]string{
			"with-peers": "true",
		})
		call.JSONCallback = secretUsageCB
		call.Do()
	},
}

func secretUsageCB(reader io.Reader, _ http.Header) {
	var data common.APISecretUsageEntries

	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if secretListOrphanOny {
		var tmp common.APISecretUsageEntries
		for _, line := range data {
			if line.LocalCount+line.RemoteCount == 0 {
				tmp = append(tmp, line)
			}
		}
		data = tmp
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].Key < data[j].Key
	})

	if secretListSortByTotal {
		sort.Slice(data, func(i, j int) bool {
			return data[i].LocalCount+data[i].RemoteCount > data[j].LocalCount+data[j].RemoteCount
		})
	}

	strData := [][]string{}
	for _, line := range data {
		d := []string{
			line.Key,
			strconv.Itoa(line.LocalCount + line.RemoteCount),
		}
		if secretListVerbose {
			d = append(d, strconv.Itoa(line.LocalCount))
			d = append(d, strconv.Itoa(line.RemoteCount))
		}

		strData = append(strData, d)
	}

	headers := []string{"Secret", "Total"}
	if secretListVerbose {
		headers = append(headers, "Local", "Remote")
	}

	client.RenderTable(headers, strData, nil)
}

func init() {
	secretCmd.AddCommand(secretUsageCmd)
	secretUsageCmd.Flags().BoolVarP(&secretListVerbose, "verbose", "v", false, "show local and remote counts")
	secretUsageCmd.Flags().BoolVarP(&secretListOrphanOny, "orphan", "o", false, "show only secrets not used by any VM")
	secretUsageCmd.Flags().BoolVarP(&secretListSortByTotal, "sort", "", false, "sort by total usage")
}
