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

// seedStatusCmd represents the "seed status" command
var seedStatusCmd = &cobra.Command{
	Use:   "status <seed-name>",
	Short: "Display seed status",
	// Long: ``,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/seed/"+args[0], map[string]string{})
		call.JSONCallback = seedStatusCB
		call.Do()
	},
}

func seedStatusCB(reader io.Reader, headers http.Header) {
	var data common.APISeedStatus
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	v := reflect.ValueOf(data)
	typeOfT := v.Type()
	for i := 0; i < v.NumField(); i++ {
		key := typeOfT.Field(i).Name
		val := common.InterfaceValueToString(v.Field(i).Interface())
		fmt.Printf("%s: %s\n", key, val)
	}
}

func init() {
	seedCmd.AddCommand(seedStatusCmd)
}
