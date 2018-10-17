package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"

	"github.com/Xfennec/mulch/common"
	"github.com/spf13/cobra"
)

// vmInfosCmd represents the "vm infos" command
var vmInfosCmd = &cobra.Command{
	Use:   "infos <vm-name>",
	Short: "Get informations about a VM",
	Long: `Return the config file used for VM creation.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/vm/infos/"+args[0], map[string]string{})
		call.JSONCallback = vmInfosDisplay
		call.Do()
	},
}

func vmInfosDisplay(reader io.Reader) {
	var data common.APIVmInfos
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
	vmCmd.AddCommand(vmInfosCmd)
}
