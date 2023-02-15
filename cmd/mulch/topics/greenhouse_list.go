package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var greenhouseListFlagBasic bool

// greenhouseListCmd represents the "greenhouse list" command
var greenhouseListCmd = &cobra.Command{
	Use:    "list",
	Short:  "List all VMs in greenhouse",
	Hidden: true,
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		greenhouseListFlagBasic, _ = cmd.Flags().GetBool("basic")
		if greenhouseListFlagBasic {
			client.GetExitMessage().Disable()
		}

		call := client.GlobalAPI.NewCall("GET", "/greenhouse", map[string]string{})
		call.JSONCallback = greenhouseListCB
		call.Do()
	},
}

func greenhouseListCB(reader io.Reader, _ http.Header) {
	var data common.APIVMListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if greenhouseListFlagBasic {
		for _, line := range data {
			fmt.Println(line.Name)
		}
	} else {
		log.Fatal("not implemented yet")
	}
}

func init() {
	greenhouseCmd.AddCommand(greenhouseListCmd)
	greenhouseListCmd.Flags().BoolP("basic", "b", false, "show basic list, without any formating")
}
