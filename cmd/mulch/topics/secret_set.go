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

When standard input is not a terminal (file, pipe), the whole input is
read, so multiline values are supported:

mulch secret set company/ssl/KEY < key.pem

In a terminal, only one line is read, unless --multiline is used: the
value is then read until EOF (Ctrl+D). In all cases, one trailing
newline is removed.

The --value flag is available for convenience but should be avoided.

Secret can be used in VM TOML files, in the "secrets" section:

secrets = [
    "company/mail/SMTP_PASSWORD",
]

Here, an environment variable named "SMTP_PASSWORD" will be injected in the VM.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		multiline, _ := cmd.Flags().GetBool("multiline")

		var value string
		if cmd.Flags().Changed("value") {
			if multiline {
				log.Fatal("--value and --multiline are mutually exclusive")
			}
			value, _ = cmd.Flags().GetString("value")
		} else {
			value = readSecretFromStdin(multiline)
		}

		call := client.GlobalAPI.NewCall("POST", "/secret/"+args[0], map[string]string{
			"value": value,
		})
		call.Do()
	},
}

func readSecretFromStdin(multiline bool) string {
	var value string

	if !term.IsTerminal(int(os.Stdin.Fd())) || multiline {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "Enter secret value, end with Ctrl+D:\n")
		}

		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err.Error())
		}
		value = string(data)
	} else {
		fmt.Fprint(os.Stderr, "Enter secret value: ")

		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			log.Fatal(err.Error())
		}
		value = line
	}

	// remove one trailing newline (\n or \r\n)
	if strings.HasSuffix(value, "\n") {
		value = strings.TrimSuffix(value, "\n")
		value = strings.TrimSuffix(value, "\r")
	}

	return value
}

func init() {
	secretSetCmd.Flags().StringP("value", "v", "", "secret value (avoid: prefer stdin)")
	secretSetCmd.Flags().BoolP("multiline", "m", false, "read value from terminal until EOF (Ctrl+D)")
	secretCmd.AddCommand(secretSetCmd)
}
