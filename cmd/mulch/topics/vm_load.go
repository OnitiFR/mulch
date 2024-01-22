package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// vmLoadCmd represents the "vm load" command
var vmLoadCmd = &cobra.Command{
	Use:   "load <vm-name>",
	Short: "Get CPU load of a VM",
	Long: `Return CPU load of a VM in percent (across all CPUs).

See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		revision, _ := cmd.Flags().GetString("revision")
		call := client.GlobalAPI.NewCall("GET", "/vm/load/"+args[0], map[string]string{
			"revision": revision,
		})
		call.JSONCallback = vmLoadDisplay
		call.Do()
	},
}

func vmLoadDisplay(reader io.Reader, _ http.Header) {
	var data common.APIVMInfos
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}
	// fmt.Printf("%+v\n", data)
	v := reflect.ValueOf(data)
	typeOfT := v.Type()
	for i := 0; i < v.NumField(); i++ {
		key := typeOfT.Field(i).Name
		val := common.InterfaceValueToString(v.Field(i).Interface())
		fmt.Printf("%s: %s\n", key, val)
	}
}

func init() {
	vmCmd.AddCommand(vmLoadCmd)
	vmLoadCmd.Flags().StringP("revision", "r", "", "revision number")
}
