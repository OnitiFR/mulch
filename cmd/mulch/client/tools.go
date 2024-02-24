package client

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}

// ParseDuration returns a duration from string (ex: 1h, 10d, 1y)
// An empty string is valid (expiration = 0)
func ParseDuration(duration string) (time.Duration, error) {
	if duration == "" || duration == "0" {
		return 0, nil
	}

	if len(duration) < 2 {
		return 0, errors.New("invalid duration format")
	}

	unit := duration[len(duration)-1:]
	value, err := strconv.Atoi(duration[:len(duration)-1])
	if err != nil {
		return 0, err
	}

	if value < 0 {
		return 0, errors.New("negative values are not allowed")
	}

	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * time.Hour * 24, nil
	case "y":
		return time.Duration(value) * time.Hour * 24 * 365, nil
	}
	return 0, errors.New("invalid duration unit (allowed: h, d, y)")
}

// DurationAsSecondsString returns a string representation of a duration (seconds)
func DurationAsSecondsString(d time.Duration) string {
	return fmt.Sprintf("%d", int(d.Seconds()))
}
