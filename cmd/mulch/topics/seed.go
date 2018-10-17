package topics

import (
	"github.com/spf13/cobra"
)

// seedCmd represents the "seed" command
var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seeds management",
	Long: `Manage seeds, the origin images used when creating a VM.
`,
}

func init() {
	rootCmd.AddCommand(seedCmd)
}
