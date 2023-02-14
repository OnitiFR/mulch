package topics

import (
	"io"
	"net/http"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// greenhouseListCmd represents the "greenhouse list" command
var greenhouseListCmd = &cobra.Command{
	Use:    "list",
	Short:  "List all VMs in greenhouse",
	Hidden: true,
	// Long: ``,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		call := client.GlobalAPI.NewCall("GET", "/greenhouse", map[string]string{})
		call.JSONCallback = greenhouseListCB
		call.Do()
	},
}

func greenhouseListCB(reader io.Reader, _ http.Header) {
	// dump whole response
	io.Copy(os.Stdout, reader)
}

func init() {
	greenhouseCmd.AddCommand(greenhouseListCmd)
}
