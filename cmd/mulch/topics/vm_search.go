package topics

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

var vmSearchFlagFailOnEmpty bool
var vmSearchFlagShowRevision bool

// vmSearchCmd represents the "vm search" command
var vmSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search VMs",
	Long: `List for one or more VMs using criteria

Some examples:
  mulch vm search 'state == "down"'
  mulch vm search '(active == false && locked == false) || revision < 5'
  mulch vm search 'like("*_prod")'
  mulch vm search 'env("APP_ENV") != ""'
  mulch vm search 'has_tag("wp-cli")'
  mulch vm search 'has_script("prepare", "deb-lamp.sh")'
  mulch vm search 'init_date < "2022-12-30"'

List of variables:
 - name (string)
 - active (bool)
 - locked (bool)
 - state (string, up/down/…)
 - author (string)
 - revision (int)
 - init_date (date)
 - seed (string)
 - cpu_count (int)
 - ram_gb (float)
 - disk_gb (float)
 - hostname (string)

List of functions:
 - like(string) bool
 - env(string) bool
 - has_domain(string) bool
 - has_script(string, string) bool (arg1: install|prepare|backup|restore, arg2: script base name)
 - has_action(string) bool
 - has_tag(string) bool

`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client.GetExitMessage().Disable()

		vmSearchFlagFailOnEmpty, _ = cmd.Flags().GetBool("fail-on-empty")
		vmSearchFlagShowRevision, _ = cmd.Flags().GetBool("show-revision")

		call := client.GlobalAPI.NewCall("GET", "/vm/search", map[string]string{
			"q":             args[0],
			"fail-on-empty": strconv.FormatBool(vmSearchFlagFailOnEmpty),
			"show-revision": strconv.FormatBool(vmSearchFlagShowRevision),
		})

		call.Do()
	},
}

func init() {
	vmCmd.AddCommand(vmSearchCmd)
	vmSearchCmd.Flags().BoolP("fail-on-empty", "f", false, "exit with error code when no VM matches")
	vmSearchCmd.Flags().BoolP("show-revision", "r", false, "show revision for each result")
}
