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

Using completion with an alias:

alias m=mulch
complete -F __start_mulch m
`,
	Run: func(cmd *cobra.Command, args []string) {
		rootCmd.GenBashCompletion(os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
