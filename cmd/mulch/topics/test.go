package topics

import (
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test <vm-name>",
	Short: "Server test call",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/test/"+args[0], map[string]string{})
		call.Do()
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
}
