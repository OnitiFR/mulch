package topics

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// completionCmd represents the "completion" command
var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generates bash completion",
	Long: `Mulch client can provide bash completion for most commands and arguments.
To load completion, run:

. <({{mulch}} completion generate)

To configure your bash shell to load completions for each session;
add this line to your ~/.bashrc or ~/.profile file.

When using multiple servers, use 'alias' options in [[server]] blocks
of ~/.mulch.toml config file to automatically generate aliases with
completion support. (don't forget to restart your shell after any change).
`,
}

func init() {
	binaryPath, _ := os.Executable()

	if os.PathSeparator == '\\' {
		binaryPath = strings.Replace(binaryPath, "\\", "/", -1)
	}

	completionCmd.Long = strings.Replace(completionCmd.Long, "{{mulch}}", binaryPath, -1)
	rootCmd.AddCommand(completionCmd)
}
