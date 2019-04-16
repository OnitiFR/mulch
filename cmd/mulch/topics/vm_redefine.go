package topics

import (
	"log"

	"github.com/spf13/cobra"
)

// vmRedefineCmd represents the "vm redefine" command
var vmRedefineCmd = &cobra.Command{
	Use:   "redefine <vm-name> <config.toml>",
	Short: "Redefine a VM",
	Long: `Redefine ("update") an existing VM with a new configuration file.

The VM will use its new configuration on next rebuild. Domain names are
immediately updated, though.

WARNING: consider this command as dangerous! Your new backup scripts may
not match your old content, for instance.

Remember: you can get current VM configuration file using "vm config <vm-name>",
it's an easy way to modify config before VM redefinition.
`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		call := globalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{"action": "redefine"})
		err := call.AddFile("config", args[1])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRedefineCmd)
}
