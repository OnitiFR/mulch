package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display server logs",
	Long: `Display all logs from the server. It may be useful to monitor
server activity, or if you need to resume VM creation after exiting
the client. All logs from all targets ("vm") are displayed.

Examples:
  mulch log
  mulch log --trace`,
	Args:    cobra.NoArgs,
	Aliases: []string{"logs"},
	Run: func(cmd *cobra.Command, args []string) {

		call := globalAPI.NewCall("GET", "/log/history", map[string]string{})
		call.JSONCallback = logCmdHistoryCB
		call.Do()

		call2 := globalAPI.NewCall("GET", "/log", map[string]string{})
		call2.DisableSpecialMessages = true
		call2.Do()
	},
}

func logCmdHistoryCB(reader io.Reader, headers http.Header) {
	fmt.Println("hello from logCmdHistoryCB")
	dec := json.NewDecoder(reader)
	for {
		var m []common.Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		// split & use printJSONStream ?
		// be sure to disable special messages, then
		fmt.Println(m)
	}
}

func init() {
	rootCmd.AddCommand(logCmd)
}
