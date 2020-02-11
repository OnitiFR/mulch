package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// seedRefreshCmd represents the "seed refresh" command
var seedRefreshCmd = &cobra.Command{
	Use:   "refresh <seed-name>",
	Short: "Refresh a seed",
	Long:  `Refresh means 'download again' for URL seeds and 'rebuild' for seeders`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/seed/"+args[0], map[string]string{
			"action": "refresh",
		})
		call.Do()
	},
}

func init() {
	seedCmd.AddCommand(seedRefreshCmd)
}
