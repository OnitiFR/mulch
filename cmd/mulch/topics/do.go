package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var doListFlagBasic bool

//  doCmd represents the "do" command
var doCmd = &cobra.Command{
	Use:   "do <vm-name> [action] [arguments]",
	Short: "Do action on VM",
	Long: `Execute a 'do action' on a VM.

If no action is given, a list of available actions for the VM will be shown.
You can give arguments to the script, but you may have to use -- for script flags.
Ex: mulch do myvm open -- -fullscreen
See [[do-actions]] in TOML description file.
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		doListFlagBasic, _ = cmd.Flags().GetBool("basic")
		revision, _ := cmd.Flags().GetString("revision")

		if doListFlagBasic == true {
			client.GetExitMessage().Disable()
		}

		if len(args) == 1 {
			call := globalAPI.NewCall("GET", "/vm/do-actions/"+args[0], map[string]string{
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

			call := globalAPI.NewCall("POST", "/vm/"+args[0], params)
			call.Do()
		}
	},
}

func doListCB(reader io.Reader, headers http.Header) {
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
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Name", "User", "Description"})
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetColWidth(50)
		table.SetCenterSeparator("|")
		table.AppendBulk(strData)
		table.Render()
	}
}

func init() {
	rootCmd.AddCommand(doCmd)
	doCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
	doCmd.Flags().StringP("revision", "r", "", "revision number")
}
