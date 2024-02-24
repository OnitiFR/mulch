package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"time"

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
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/seed/"+args[0], map[string]string{})
		call.JSONCallback = seedStatusCB
		call.Do()
	},
}

func seedStatusCB(reader io.Reader, _ http.Header) {
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

		if key == "PausedUntil" {
			continue
		}

		val := common.InterfaceValueToString(v.Field(i).Interface())
		fmt.Printf("%s: %s\n", key, val)
	}

	if data.PausedUntil.After(time.Now()) {
		fmt.Printf("PausedUntil: %s\n", data.PausedUntil.Format("2006-01-02 15:04:05"))
	}

}

func init() {
	seedCmd.AddCommand(seedStatusCmd)
}
