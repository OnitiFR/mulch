package topics

import (
	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// secretSetCmd represents the "secret set" command
var secretSetCmd = &cobra.Command{
	Use:   "set <name> <value>",
	Short: "Set a secret value",
	Long: `Create or update a secret value.

Secret can be used in VM TOML files, in the "secrets" section:

secrets = [
    "company/mail/SMTP_PASSWORD",
]

Here, an environment variable named "SMTP_PASSWORD" will be injected in the VM.
`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		call := client.GlobalAPI.NewCall("POST", "/secret/"+args[0], map[string]string{
			"value": args[1],
		})
		call.Do()
	},
}

func init() {
	secretCmd.AddCommand(secretSetCmd)
}
