package topics

import (
	"log"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/spf13/cobra"
)

// seedPauseCmd represents the "seed pause" command
var seedPauseCmd = &cobra.Command{
	Use:   "pause <seed-name>",
	Short: "Pause a seed refresh",
	Long: `Pause seed auto-refresh for a given duration

Allowed units:  h, d, y (hours, days, years)
Allowed values: any positive integer

Give an empty string (or 0) to remove the pause.

Examples:
	mulch seed pause my-seed 3h
	mulch seed pause my-seed 0
`,

	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		dur, err := client.ParseDuration(args[1])
		if err != nil {
			log.Fatalf("unable to parse duration: %s", err)
		}

		call := client.GlobalAPI.NewCall("POST", "/seed/"+args[0], map[string]string{
			"action":   "pause",
			"duration": client.DurationAsSecondsString(dur),
		})
		call.Do()
	},
}

func init() {
	seedCmd.AddCommand(seedPauseCmd)
}
