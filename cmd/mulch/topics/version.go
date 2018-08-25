package topics

import (
	"fmt"
	"strconv"

	"github.com/Xfennec/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display versions",
	Long: `Display client and protocol versions. You can also add
server version to the result.

Examples:
  mulch version
  mulch version --server`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("client version: %s\n", client.Version)
		fmt.Printf("client protocol: %s\n", strconv.Itoa(client.ProtocolVersion))

		server, _ := cmd.Flags().GetBool("server")
		if server {
			call := globalAPI.NewCall("GET", "/version", map[string]string{})
			call.Do()
		}

	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolP("server", "s", false, "also show server version")
}
