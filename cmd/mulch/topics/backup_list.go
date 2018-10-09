package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/Xfennec/mulch/common"
	"github.com/c2h5oh/datasize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// backupListCmd represents the "backup list" command
var backupListCmd = &cobra.Command{
	Use:   "list [vm-name]",
	Short: "List backups",
	// Long: ``,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		vmFilter := ""
		if len(args) > 0 {
			vmFilter = args[0]
		}
		call := globalAPI.NewCall("GET", "/backup", map[string]string{
			"vm": vmFilter,
		})
		call.JSONCallback = backupListCB
		call.Do()
	},
}

func backupListCB(reader io.Reader) {
	var data common.APIBackupListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("No result. You may use 'mulch vm <vm-name> backup'.\n")
		return
	}

	strData := [][]string{}
	for _, line := range data {
		strData = append(strData, []string{
			line.VMName,
			// line.Created.Format(time.RFC3339),
			line.DiskName,
			// (datasize.ByteSize(line.Size) * datasize.B).HR(),
			(datasize.ByteSize(line.AllocSize) * datasize.B).HR(),
		})
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"VM", "Disk Name", "Size"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(strData)
	table.Render()
}

func init() {
	backupCmd.AddCommand(backupListCmd)
}
