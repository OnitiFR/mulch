package topics

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// store expression?

// vmSearchCmd represents the "vm search" command
var vmSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search VMs",
	Long: `List for one or more VMs using criteria

ex: mulch vm search "(active = true && locked = false) or revision > 10"

List of criteria:
 - active (bool)
 - locked (bool)
 - name (string)
 - author (string)
 - revision (int)
 - init_date (date)
 - seed (string)
 - domains ?
 - cpu_count
 - ram ? disk ?
 - state ? (up/down)
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client.GetExitMessage().Disable()

		// var expr *govaluate.EvaluableExpression

		// task_result.go, 70:
		// res, err := check.If.Evaluate(params)

		// config_probe.go, 178:
		// expr, err := govaluate.NewEvaluableExpressionWithFunctions(tProbe.RunIf, CheckFunctions

		// check_functions.go

		call := client.GlobalAPI.NewCall("GET", "/vm", map[string]string{})
		call.JSONCallback = vmSearchCB
		call.Do()
	},
}

func vmSearchCB(reader io.Reader, headers http.Header) {
	var data common.APIVMListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	// if len(results) == 0 {
	// 	// exit code!
	// 	return
	// }

}

func init() {
	vmCmd.AddCommand(vmSearchCmd)
}
