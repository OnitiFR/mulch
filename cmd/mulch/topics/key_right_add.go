package topics

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// keyRightAddCmd represents the "key right add" command
var keyRightAddCmd = &cobra.Command{
	Use:   "add <key> <right> | add <key> --file <file>",
	Short: "Add right(s) to the key",
	Long: `Add a right to the key, or multiple rights at once with --file

With --file (use "-" for stdin), rights are read one per line, empty lines
and lines starting with "#" are ignored. This is atomic: if any right
is invalid, nothing is added. Existing rights are skipped.

The right must follow this format:
METHOD path header1=value1 header2=value2

You can use "*" joker in method, path, and header values.

WARNING: no rights means full privileges.

-- Examples:
Allow to list VMs:
GET /vm

Allow VM backup:
POST /vm/* action=backup

Allow to run a specific action to a specific VM:
POST /vm/myvm action=do do_action=logs

Allow SSH access:
GET /sshpair
SSH /myvm

Allow VM creation:
POST /vm
CREATE /vm/*

Copy all rights from a key to another:
mulch key right list alice | mulch key right add bob -f -

Note: when setting rights, server log shows denied requests, it may help you.
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("file") {
			return cobra.ExactArgs(1)(cmd, args)
		}
		return cobra.ExactArgs(2)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		params := make(map[string]string)

		if cmd.Flags().Changed("file") {
			file, _ := cmd.Flags().GetString("file")
			params["rights"] = readRightsFile(file)
		} else {
			params["right"] = args[1]
		}

		call := client.GlobalAPI.NewCall("POST", "/key/right/"+args[0], params)
		call.Do()
	},
}

func readRightsFile(file string) string {
	var data []byte
	var err error

	if file == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		log.Fatal(fmt.Errorf("no right found in '%s'", file))
	}

	return string(data)
}

func init() {
	keyRightAddCmd.Flags().StringP("file", "f", "", "read rights from a file, one per line (\"-\" for stdin)")
	keyRightCmd.AddCommand(keyRightAddCmd)
}
