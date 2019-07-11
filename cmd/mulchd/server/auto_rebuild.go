package server

import (
	"time"
)

// AutoRebuildSchedule will schedule auto-rebuilds
func AutoRebuildSchedule(app *App) {
	// big relaxing, so we don't compete too much with VM state restore,
	// seed updating and other mulchd startup stuff.
	time.Sleep(5 * time.Minute)

	for {
		now := time.Now().Format("15:04")
		if app.Config.AutoRebuildTime == now {
			autoRebuildStart(app)
		}
		time.Sleep(time.Minute)
	}
}

func autoRebuildStart(app *App) {
	// TODO:
	// - add VM info LastRebuildDate, LastRebuildDuration (+ export VM status)
	// - loop VMs, check last rebuild, rebuild if needed
	app.Log.Info("auto-rebuild")
}
