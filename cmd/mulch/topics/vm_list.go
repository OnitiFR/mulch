package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"log"

	"github.com/spf13/cobra"
	"github.com/Xfennec/mulch/common"
	"github.com/olekukonko/tablewriter"
	"github.com/fatih/color"
)

// vmListCmd represents the vmList command
var vmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/vm", map[string]string{})
		call.JSONCallback = vmListCB
		call.Do()
	},
}

func vmListCB(reader io.Reader) {
	var data common.APIVmListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("Currently, no VM exists. You may use 'mulch vm create'.\n")
		return
	}

	strData := [][]string{}
	red := color.New(color.FgHiRed).SprintFunc()
	green := color.New(color.FgHiGreen).SprintFunc()
	yellow := color.New(color.FgHiYellow).SprintFunc()
	for _, line := range data {
		state := red(line.State)
		if line.State == "up" {
			state = green(line.State)
		}

		locked := "false"
		if line.Locked == true {
			locked = yellow("locked")
		}

		strData = append(strData, []string {
			line.Name,
			line.LastIP,
			state,
			locked,
		})
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Last known IP", "State", "Locked"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(strData)
	table.Render()
}

func init() {
	vmCmd.AddCommand(vmListCmd)
}
