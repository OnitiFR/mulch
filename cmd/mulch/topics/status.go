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

// statusCmd represents the "status" command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get informations about Mulchd host",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		call := client.GlobalAPI.NewCall("GET", "/status", map[string]string{})
		call.JSONCallback = statusDisplay
		call.Do()
	},
}

func statusDisplay(reader io.Reader, headers http.Header) {
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
		if key != "SSHConnections" && key != "Operations" {
			fmt.Printf("%s: %s\n", key, val)
		}
	}

	referenceTime := time.Now()
	if headers.Get("Date") != "" {
		date, err := http.ParseTime(headers.Get("Date"))
		if err == nil {
			referenceTime = date
		}
	}

	fmt.Printf("SSHConnections: %d\n", len(data.SSHConnections))
	for _, conn := range data.SSHConnections {
		since := referenceTime.Sub(conn.StartTime)
		fmt.Printf(" - from %s@%s to %s@%s (%s)\n",
			conn.FromUser,
			conn.FromIP,
			conn.ToUser,
			conn.ToVMName,
			since)
	}

	fmt.Printf("Operations: %d\n", len(data.Operations))
	for _, op := range data.Operations {
		since := referenceTime.Sub(op.StartTime)
		fmt.Printf(" - from %s: %s %s %s (%s)\n",
			op.Origin,
			op.Action,
			op.Ressource,
			op.RessourceName,
			since,
		)
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
