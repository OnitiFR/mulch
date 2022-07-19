package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyCreateCmd represents the "key create" command
var keyCreateCmd = &cobra.Command{
	Use:   "create <key-comment>",
	Short: "Create an API key",
	Long: `Create an API key used by one or more clients to access Mulchd.

The comment can be a user name or anything that will help you to manage
your keys. It must not be duplicated.

The key will be displayed by this command but will NOT be visible anymore
after. The only option left will be to look at the daemon key database directly.
`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/key", map[string]string{
			"comment": args[0],
		})
		call.Do()
	},
}

func init() {
	keyCmd.AddCommand(keyCreateCmd)
}
