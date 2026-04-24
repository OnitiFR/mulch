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

var replicationListFlagBasic bool

// replicationListCmd represents the "replication list" command
var replicationListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List replicated VMs",
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		replicationListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if replicationListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/replication", map[string]string{})
		call.JSONCallback = replicationListCB
		call.Do()
	},
}

func replicationListCB(reader io.Reader, _ http.Header) {
	var data common.APIReplicationEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if replicationListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Name)
		}
		return
	}

	if len(data) == 0 {
		fmt.Printf("No replicated VM.\n")
		return
	}

	red := color.New(color.FgHiRed).SprintFunc()
	green := color.New(color.FgHiGreen).SprintFunc()
	yellow := color.New(color.FgHiYellow).SprintFunc()
	grey := color.New(color.FgHiBlack).SprintFunc()

	strData := [][]string{}
	now := time.Now()
	for _, line := range data {
		status := line.Status
		switch line.Status {
		case "idle":
			status = green(status)
		case "syncing":
			status = yellow(status)
		case "error":
			status = red(status)
		}

		full := green("yes")
		if !line.FullCopyDone {
			full = red("no")
		}

		lastSync := grey("never")
		if !line.LastSyncTime.IsZero() {
			lastSync = client.HumanDuration(now.Sub(line.LastSyncTime)) + " ago"
		}

		duration := grey("-")
		if line.LastSyncDuration > 0 {
			duration = client.HumanShortDuration(line.LastSyncDuration)
		}

		size := grey("-")
		if line.LastSyncBytes > 0 {
			size = client.HumanBytes(line.LastSyncBytes)
		}

		errs := "0"
		if line.ConsecutiveErrors > 0 {
			errs = red(strconv.Itoa(line.ConsecutiveErrors))
		}

		name := line.Name
		if !line.Active {
			name = grey(name)
		}

		strData = append(strData, []string{
			name,
			strconv.Itoa(line.Revision),
			line.PeerName,
			status,
			full,
			lastSync,
			duration,
			size,
			errs,
		})
	}

	headers := []string{"Name", "Rev", "Peer", "Status", "Full", "Last Sync", "Duration", "Last Bytes", "Errors"}
	client.RenderTable(headers, strData)
}

func init() {
	replicationCmd.AddCommand(replicationListCmd)
	replicationListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
