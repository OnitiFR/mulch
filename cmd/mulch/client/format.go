package client

import (
	"fmt"
	"time"

	"github.com/c2h5oh/datasize"
)

// HumanDuration renders a duration with coarse, single-unit granularity
// (e.g. "42s", "5m", "3h", "2d"). Suited for "X ago" displays.
func HumanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// HumanShortDuration renders a duration with precision adapted to its
// magnitude: milliseconds when below a second, tenths of seconds below a
// minute, and whole seconds beyond. Suited for "sync took X" displays.
func HumanShortDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	if d < time.Minute {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

// HumanBytes renders a byte count using a human-readable unit suffix
// (KB, MB, GB, ...).
func HumanBytes(bytes uint64) string {
	return (datasize.ByteSize(bytes) * datasize.B).HR()
}
