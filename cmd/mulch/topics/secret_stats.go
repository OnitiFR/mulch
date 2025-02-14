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

// secretStartsCmd represents the "secret stats" command
var secretStartsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display secret stats",

	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("GET", "/secret-stats", map[string]string{})
		call.JSONCallback = secretStatsCB
		call.Do()
	},
}

func secretStatsCB(reader io.Reader, _ http.Header) {
	var data common.APISecretStats
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
	secretCmd.AddCommand(secretStartsCmd)
}
