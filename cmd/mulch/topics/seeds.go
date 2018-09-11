package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/Xfennec/mulch/common"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// vmSeedsCmd represents the vmSeeds command
var vmSeedsCmd = &cobra.Command{
	Use:   "seeds",
	Short: "List all Seeds",
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/seed", map[string]string{})
		call.JSONCallback = vmSeedsCB
		call.Do()
	},
}

func vmSeedsCB(reader io.Reader) {
	var data common.APISeedListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("Currently, no seed exists. You must declare seeds in your Mulch server.\n")
		return
	}

	strData := [][]string{}
	red := color.New(color.FgHiRed).SprintFunc()
	green := color.New(color.FgHiGreen).SprintFunc()
	for _, line := range data {
		state := red("not-ready")
		if line.Ready == true {
			state = green("ready")
		}

		strData = append(strData, []string{
			line.Name,
			state,
			line.LastModified.Format(time.RFC3339),
		})
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Ready", "Last modified"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(strData)
	table.Render()
}

func init() {
	rootCmd.AddCommand(vmSeedsCmd)
}
