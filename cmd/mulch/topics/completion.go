package topics

import (
	"os"

	"github.com/spf13/cobra"
)

// completionCmd represents the "completion" command
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generates bash completion scripts",
	Long: `To load completion run

. <(mulch completion)

To configure your bash shell to load completions for each session;
add this line to your ~/.bashrc or ~/.profile file.

When using multiple servers, use 'alias' options in [[server]] blocks
of ~/.mulch.toml config file to automatically generate aliases with
completion support. (don't forget to restart your shell after any change).
`,
	Run: func(cmd *cobra.Command, args []string) {
		rootCmd.GenBashCompletion(os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
