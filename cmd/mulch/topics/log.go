package topics

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

const logCmdDefaultLines = 20

var logCmdWithTarget = false

var logCmd = &cobra.Command{
	Use:   "log [target]",
	Short: "Display server logs",
	Long: `Display logs from the server. It may be useful to monitor
server activity, or if you need to resume VM creation after exiting
the client. You can choose a specific target ("vm").

Message timestamps are always displayed with this command.
(--time is forced, in other words.)

Examples:
  mulch log -f
  mulch log my_vm
  mulch log --trace`,
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"logs"},
	Run: func(cmd *cobra.Command, args []string) {

		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")
		target := common.MessageAllTargets
		if len(args) > 0 {
			target = args[0]
			logCmdWithTarget = true
		}

		call := globalAPI.NewCall("GET", "/log/history", map[string]string{
			"target": target,
			"lines":  strconv.Itoa(lines),
		})
		call.JSONCallback = logCmdHistoryCB
		call.Do()

		if follow {
			call2 := globalAPI.NewCall("GET", "/log", map[string]string{
				"target": target,
			})
			call2.DisableSpecialMessages = true
			call2.TimestampShow(true)
			call2.PrintLogTarget = !logCmdWithTarget
			call2.Do()
		}
	},
}

func logCmdHistoryCB(reader io.Reader, headers http.Header) {
	dec := json.NewDecoder(reader)
	var messages []common.Message
	err := dec.Decode(&messages)
	if err != nil {
		log.Fatal(err)
	}
	for _, message := range messages {
		message.Print(common.MessagePrintTime, !logCmdWithTarget)
	}
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().IntP("lines", "n", logCmdDefaultLines, "display n lines")
	logCmd.Flags().BoolP("follow", "f", false, "follow live log")
}
