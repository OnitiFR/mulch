package topics

import (
	"fmt"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display versions",
	Long: `Display client and protocol versions. You can also add
server version to the result.

Examples:
  mulch version
  mulch version --remote`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("client version: %s\n", client.Version)
		fmt.Printf("client protocol: %s\n", strconv.Itoa(client.ProtocolVersion))

		server, _ := cmd.Flags().GetBool("remote")
		if server {
			call := globalAPI.NewCall("GET", "/version", map[string]string{})
			call.Do()
		}

	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolP("remote", "r", false, "also show server version")
}
