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
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// replicationStatusCmd represents the "replication status" command
var replicationStatusCmd = &cobra.Command{
	Use:   "status <vm-name>",
	Short: "Show replication status for a VM",
	Long: `Show detailed replication status for a VM, including last error.

See 'replication list' for replicated VM names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("GET", "/replication/"+args[0], map[string]string{
			"revision": revision,
		})
		call.JSONCallback = replicationStatusCB
		call.Do()
	},
}

func replicationStatusCB(reader io.Reader, _ http.Header) {
	var data common.APIReplicationEntry
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	red := color.New(color.FgHiRed).SprintFunc()
	green := color.New(color.FgHiGreen).SprintFunc()
	yellow := color.New(color.FgHiYellow).SprintFunc()
	grey := color.New(color.FgHiBlack).SprintFunc()

	status := data.Status
	switch data.Status {
	case "idle":
		status = green(status)
	case "syncing":
		status = yellow(status)
	case "error":
		status = red(status)
	case "stand-down":
		// the VM was promoted on the peer: replication (and the VM) stopped
		status = red(status)
	}

	full := green("yes")
	if !data.FullCopyDone {
		full = red("no")
	}

	now := time.Now()

	lastSync := grey("never")
	if !data.LastSyncTime.IsZero() {
		lastSync = data.LastSyncTime.Format("2006-01-02 15:04:05") +
			" (" + client.HumanDuration(now.Sub(data.LastSyncTime)) + " ago)"
	}

	duration := grey("-")
	if data.LastSyncDuration > 0 {
		duration = client.HumanShortDuration(data.LastSyncDuration)
	}

	size := grey("-")
	if data.LastSyncBytes > 0 {
		size = client.HumanBytes(data.LastSyncBytes)
	}

	errs := "0"
	if data.ConsecutiveErrors > 0 {
		errs = red(fmt.Sprintf("%d", data.ConsecutiveErrors))
	}

	lastError := grey("none")
	if data.LastError != "" {
		lastError = red(data.LastError)
		if !data.LastErrorTime.IsZero() {
			lastError += fmt.Sprintf("\n                    %s (%s ago)",
				data.LastErrorTime.Format("2006-01-02 15:04:05"),
				client.HumanDuration(now.Sub(data.LastErrorTime)))
		}
	}

	activeStr := green("yes")
	if !data.Active {
		activeStr = grey("no")
	}

	fmt.Printf("Name:               %s\n", data.Name)
	fmt.Printf("Revision:           %d\n", data.Revision)
	fmt.Printf("Active:             %s\n", activeStr)
	fmt.Printf("Peer:               %s\n", data.PeerName)
	fmt.Printf("Status:             %s\n", status)
	fmt.Printf("Full copy done:     %s\n", full)
	fmt.Printf("Last sync:          %s\n", lastSync)
	fmt.Printf("Last sync duration: %s\n", duration)
	fmt.Printf("Last sync bytes:    %s\n", size)
	fmt.Printf("Consecutive errors: %s\n", errs)

	if data.BackoffInterval > data.ConfiguredInterval && data.ConfiguredInterval > 0 {
		fmt.Printf("Backoff:            %s, effective interval: %s (configured: %s)\n",
			red("active"),
			client.HumanShortDuration(data.BackoffInterval),
			client.HumanShortDuration(data.ConfiguredInterval))
	}

	alertedStr := green("no")
	if data.Alerted {
		alertedStr = red("yes")
	}
	fmt.Printf("Alerted:            %s\n", alertedStr)

	if data.AlertDelay > 0 {
		fmt.Printf("Alert after:        %s without successful sync\n",
			client.HumanShortDuration(data.AlertDelay))
	}

	fmt.Printf("Last error:         %s\n", lastError)
}

func init() {
	replicationCmd.AddCommand(replicationStatusCmd)
	replicationStatusCmd.Flags().StringP("revision", "r", "", "revision number")
}
