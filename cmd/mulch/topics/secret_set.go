package topics

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// secretSetCmd represents the "secret set" command
var secretSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set a secret value",
	Long: `Create or update a secret value.

The value is read from standard input (so it does not leak in the
shell history or in the process list).

The --value flag is available for convenience but should be avoided.

Secret can be used in VM TOML files, in the "secrets" section:

secrets = [
    "company/mail/SMTP_PASSWORD",
]

Here, an environment variable named "SMTP_PASSWORD" will be injected in the VM.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var value string
		if cmd.Flags().Changed("value") {
			value, _ = cmd.Flags().GetString("value")
		} else {
			value = readSecretFromStdin()
		}

		call := client.GlobalAPI.NewCall("POST", "/secret/"+args[0], map[string]string{
			"value": value,
		})
		call.Do()
	},
}

func readSecretFromStdin() string {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Enter secret value: ")
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		log.Fatal(err.Error())
	}

	return strings.TrimRight(line, "\r\n")
}

func init() {
	secretSetCmd.Flags().StringP("value", "v", "", "secret value (avoid: prefer stdin)")
	secretCmd.AddCommand(secretSetCmd)
}
