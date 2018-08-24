package topics

import (
	"github.com/spf13/cobra"
)

var test2Cmd = &cobra.Command{
	Use:   "test2",
	Short: "call 2nd server test",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// api := client.NewAPI(viper.GetString("url"), viper.GetBool("trace"))
		call := globalAPI.NewCall("POST", "/test2", map[string]string{})
		call.Do()
	},
}

func init() {
	rootCmd.AddCommand(test2Cmd)
}
