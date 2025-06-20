package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var vmListFlagBasic bool

// vmListCmd represents the "vm list" command
var vmListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all VMs",
	Aliases: []string{"ls"},
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		vmListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if vmListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/vm", map[string]string{
			"basic": strconv.FormatBool(vmListFlagBasic),
		})
		call.JSONCallback = vmListCB
		call.Do()
	},
}

func vmListCB(reader io.Reader, _ http.Header) {
	var data common.APIVMListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if vmListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Name)
		}
	} else {
		if len(data) == 0 {
			fmt.Printf("Currently, no VM exists. You may use 'mulch vm create'.\n")
			return
		}

		strData := [][]string{}
		red := color.New(color.FgHiRed).SprintFunc()
		green := color.New(color.FgHiGreen).SprintFunc()
		yellow := color.New(color.FgHiYellow).SprintFunc()
		grey := color.New(color.FgHiBlack).SprintFunc()
		for _, line := range data {
			state := red(line.State)
			if line.State == "up" {
				state = green(line.State)
			}

			locked := "false"
			if line.Locked {
				locked = yellow("locked")
			}

			name := line.Name
			if !line.Active {
				name = grey(name)
			}

			strData = append(strData, []string{
				name,
				strconv.Itoa(line.Revision),
				state,
				locked,
				yellow(line.WIP),
			})
		}

		headers := []string{"Name", "Rev", "State", "Locked", "Operation"}
		client.RenderTable(headers, strData, nil)
	}
}

func init() {
	vmCmd.AddCommand(vmListCmd)
	vmListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
