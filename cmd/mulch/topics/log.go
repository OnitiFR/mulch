package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Xfennec/mulch"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "display server logs",
	Long: `Display all logs from the server. It may be useful to monitor
server activity, or if you need to resume VM creation after exiting
the client. All logs from all targets ("vm") are displayed.

Examples:
  mulch log
  mulch log --trace`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		trace, _ := cmd.Flags().GetBool("trace")

		params := ""
		if trace == true {
			params = "?trace=true"
		}

		req, err := http.NewRequest("GET", viper.GetString("url")+"/log"+params, nil)
		if err != nil {
			log.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("Status code is not OK: %v (%s)", resp.StatusCode, resp.Status)
		}

		dec := json.NewDecoder(resp.Body)
		for {
			var m mulch.Message
			err := dec.Decode(&m)
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}
			fmt.Printf("%s: %s\n", m.Type, m.Message)
		}

	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().BoolP("trace", "t", false, "also show TRACE messages (debug)")
}
