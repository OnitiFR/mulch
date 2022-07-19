package topics

import (
	"os"

	"github.com/spf13/cobra"
)

// completionGenerateCmd represents the "completion generate" command
var completionGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Actually generates bash completion (see 'mulch completion')",
	Run: func(_ *cobra.Command, _ []string) {
		rootCmd.GenBashCompletion(os.Stdout)
	},
}

func init() {
	completionCmd.AddCommand(completionGenerateCmd)
}
