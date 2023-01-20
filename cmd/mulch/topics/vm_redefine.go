package topics

import (
	"log"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// vmRedefineCmd represents the "vm redefine" command
var vmRedefineCmd = &cobra.Command{
	Use:   "redefine <vm-name> <config.toml>",
	Short: "Redefine a VM",
	Long: `Redefine ("update") an existing VM with a new configuration file.

The VM will use its new configuration on next rebuild, with a few exceptions:
  * domain names and ports are immediately updated
  * environment variables and secrets only require a VM restart

WARNING: consider this command dangerous! Your new backup scripts may
not match your old content, for instance.

Remember: you can get current VM configuration file using "vm config <vm-name>",
it's an easy way to get and modify config before VM redefinition.
`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		revision, _ := cmd.Flags().GetString("revision")

		call := client.GlobalAPI.NewCall("POST", "/vm/"+args[0], map[string]string{
			"action":   "redefine",
			"force":    strconv.FormatBool(force),
			"revision": revision,
		})
		err := call.AddFile("config", args[1])
		if err != nil {
			log.Fatal(err)
		}
		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmRedefineCmd)
	vmRedefineCmd.Flags().BoolP("force", "f", false, "force redefine on a locked VM")
	vmRedefineCmd.Flags().StringP("revision", "r", "", "revision number")
}
