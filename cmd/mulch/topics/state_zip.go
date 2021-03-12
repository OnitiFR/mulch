package topics

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// stateZipCmd represents the "state zip" command
var stateZipCmd = &cobra.Command{
	Use:   "zip",
	Short: "Download a zip file with VMs configs and states",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path, _ := cmd.Flags().GetString("path")
		filename := fmt.Sprintf("mulch-state-%s-%s.zip",
			client.GlobalConfig.Server.Name,
			time.Now().Format("20060102-150405"),
		)

		call := client.GlobalAPI.NewCall("GET", "/state/zip", map[string]string{})
		call.DestFilePath = filepath.Clean(fmt.Sprintf("%s/%s", path, filename))
		call.Do()
	},
}

func init() {
	stateCmd.AddCommand(stateZipCmd)
	stateZipCmd.Flags().StringP("path", "p", "./", "output path")
}
