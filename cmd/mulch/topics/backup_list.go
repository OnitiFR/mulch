package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var backupListFlagBasic bool

// backupListCmd represents the "backup list" command
var backupListCmd = &cobra.Command{
	Use:   "list [vm-name]",
	Short: "List backups",
	// Long: ``,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backupListFlagBasic, _ = cmd.Flags().GetBool("basic")

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

func backupListCB(reader io.Reader, headers http.Header) {
	var data common.APIBackupListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if backupListFlagBasic {
		for _, line := range data {
			fmt.Println(line.DiskName)
		}
	} else {
		if len(data) == 0 {
			fmt.Printf("No result. You may use 'mulch vm <vm-name> backup'.\n")
			return
		}

		strData := [][]string{}
		for _, line := range data {
			strData = append(strData, []string{
				line.DiskName,
				// line.VMName,
				line.AuthorKey,
				// line.Created.Format(time.RFC3339),
				// (datasize.ByteSize(line.Size) * datasize.B).HR(),
				(datasize.ByteSize(line.AllocSize) * datasize.B).HR(),
			})
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Disk Name", "Author", "Size"})
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetCenterSeparator("|")
		table.AppendBulk(strData)
		table.Render()
	}
}

func init() {
	backupCmd.AddCommand(backupListCmd)
	backupListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
