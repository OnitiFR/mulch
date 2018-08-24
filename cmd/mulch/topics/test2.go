package topics

import (
	"github.com/Xfennec/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var test2Cmd = &cobra.Command{
	Use:   "test2",
	Short: "call 2nd server test",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		trace, _ := cmd.Flags().GetBool("trace")
		api := client.NewAPI(viper.GetString("url"), trace)
		call := api.NewCall("POST", "/test2", map[string]string{})
		call.Do()
	},
}

func init() {
	rootCmd.AddCommand(test2Cmd)
	test2Cmd.Flags().BoolP("trace", "t", false, "also show TRACE messages (debug)")
}
