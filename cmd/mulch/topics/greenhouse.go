package topics

import (
	"github.com/spf13/cobra"
)

// greenhouseCmd represents the "seed" command
var greenhouseCmd = &cobra.Command{
	Use:   "greenhouse",
	Short: "Greenhouse management",
	Long: `Manage greenhouse, the place where new VMs are built.
`,
}

func init() {
	rootCmd.AddCommand(greenhouseCmd)
}
