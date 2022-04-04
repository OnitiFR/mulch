package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
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
		if backupListFlagBasic {
			client.GetExitMessage().Disable()
		}

		vmFilter := ""
		if len(args) > 0 {
			vmFilter = args[0]
		}
		call := client.GlobalAPI.NewCall("GET", "/backup", map[string]string{
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
			expires := ""
			if !line.Expire.IsZero() {
				expires = line.Expire.Format("2006-01-02 15:04")
			}
			strData = append(strData, []string{
				line.DiskName,
				// line.VMName,
				line.AuthorKey,
				// line.Created.Format(time.RFC3339),
				// (datasize.ByteSize(line.Size) * datasize.B).HR(),
				(datasize.ByteSize(line.AllocSize) * datasize.B).HR(),
				expires,
			})
		}

		headers := []string{"Disk Name", "Author", "Size", "Expires"}
		client.RenderTableTruncateCol(0, headers, strData)
	}
}

func init() {
	backupCmd.AddCommand(backupListCmd)
	backupListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
