package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var doListFlagBasic bool

// doCmd represents the "do" command
var doCmd = &cobra.Command{
	Use:   "do <vm-name> [action] [arguments]",
	Short: "Do action on VM",
	Long: `Execute a 'do action' on a VM.

If no action is given, a list of available actions for the VM will be shown.
You can gives arguments to the script, but you may have to use -- separator.

Ex: mulch do myvm open -- -fullscreen

Notes:
- see [[do-actions]] in TOML description file, and _MULCH_ACTION* special lines
in prepare scripts
- environment _CALLING_KEY is injected in the script
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		doListFlagBasic, _ = cmd.Flags().GetBool("basic")
		revision, _ := cmd.Flags().GetString("revision")

		if doListFlagBasic {
			client.GetExitMessage().Disable()
		}

		if len(args) == 1 {
			call := client.GlobalAPI.NewCall("GET", "/vm/do-actions/"+args[0], map[string]string{
				"revision": revision,
			})
			call.JSONCallback = doListCB
			call.Do()
		} else {
			arguments := strings.Join(args[2:], " ")

			params := map[string]string{
				"action":    "do",
				"do_action": args[1],
				"arguments": arguments,
				"revision":  revision,
			}

			call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], params)
			call.Do()
		}
	},
}

func doListCB(reader io.Reader, _ http.Header) {
	var data common.APIVMDoListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if doListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Name)
		}
	} else {
		if len(data) == 0 {
			fmt.Printf("Currently, no 'do action' exists, see [[do-ations]] section of VM TOML file.\n")
			return
		}

		strData := [][]string{}
		for _, line := range data {
			strData = append(strData, []string{
				line.Name,
				line.User,
				line.Description,
			})
		}

		headers := []string{"Name", "User", "Description"}
		client.RenderTable(headers, strData)

	}
}

func init() {
	rootCmd.AddCommand(doCmd)
	doCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
	doCmd.Flags().StringP("revision", "r", "", "revision number")
}
