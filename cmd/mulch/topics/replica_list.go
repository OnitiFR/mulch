package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var replicaListFlagBasic bool

// replicaListCmd represents the "replica list" command
var replicaListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List replicas held by this peer",
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		replicaListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if replicaListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/replica", map[string]string{})
		call.JSONCallback = replicaListCB
		call.Do()
	},
}

func replicaListCB(reader io.Reader, _ http.Header) {
	var data common.APIReplicaEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if replicaListFlagBasic {
		for _, line := range data {
			fmt.Println(replicaID(line.Name, line.Revision))
		}
		return
	}

	if len(data) == 0 {
		fmt.Printf("No replica.\n")
		return
	}

	green := color.New(color.FgHiGreen).SprintFunc()
	yellow := color.New(color.FgHiYellow).SprintFunc()
	red := color.New(color.FgHiRed).SprintFunc()
	grey := color.New(color.FgHiBlack).SprintFunc()

	strData := [][]string{}
	now := time.Now()
	for _, line := range data {
		status := line.Status
		switch line.Status {
		case "syncing", "promoting":
			status = yellow(status)
		case "promoted":
			// tombstone: the disk is gone, the entry only blocks the source
			// peer from replicating this name again
			status = grey(status)
		case "idle":
			// an idle replica whose full copy never completed is not coherent
			// and must not be promoted: surface it as a distinct state.
			if line.ConsistentSnapshot {
				status = green(status)
			} else {
				status = red("incomplete")
			}
		}

		lastUpdate := grey("never")
		if !line.LastUpdate.IsZero() {
			lastUpdate = client.HumanDuration(now.Sub(line.LastUpdate)) + " ago"
		}

		size := grey("-")
		if line.DiskSize > 0 {
			size = client.HumanBytes(line.DiskSize)
		}

		lastBytes := grey("-")
		if line.LastSyncBytes > 0 {
			lastBytes = client.HumanBytes(line.LastSyncBytes)
		}

		origin := line.Origin
		if origin == "" {
			origin = grey("-")
		}

		strData = append(strData, []string{
			line.Name,
			strconv.Itoa(line.Revision),
			origin,
			status,
			size,
			lastUpdate,
			lastBytes,
		})
	}

	headers := []string{"Name", "Rev", "Origin", "Status", "Disk Size", "Last Update", "Last Bytes"}
	client.RenderTable(headers, strData)
}

func init() {
	replicaCmd.AddCommand(replicaListCmd)
	replicaListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
