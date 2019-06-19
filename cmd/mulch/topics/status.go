package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"
	"time"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// statusCmd represents the "status" command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get informations about Mulchd host",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("GET", "/status", map[string]string{})
		call.JSONCallback = statusDisplay
		call.Do()
	},
}

func statusDisplay(reader io.Reader) {
	var data common.APIStatus
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
		if key != "SSHConnections" {
			fmt.Printf("%s: %s\n", key, val)
		}
	}

	fmt.Printf("SSHConnections: %d\n", len(data.SSHConnections))
	for _, conn := range data.SSHConnections {
		since := time.Now().Sub(conn.StartTime)
		fmt.Printf(" - from %s@%s to %s@%s (%s)\n",
			conn.FromUser,
			conn.FromIP,
			conn.ToUser,
			conn.ToVMName,
			since)
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
