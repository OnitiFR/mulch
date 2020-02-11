package server

import (
	"fmt"
	"time"
)

// AutoRebuildSchedule will schedule auto-rebuilds
func AutoRebuildSchedule(app *App) {
	app.VMStateDB.WaitRestore()
	// big relaxing, so we don't compete too much with other things like
	// seeder rebuilds and other mulchd startup stuff.
	time.Sleep(15 * time.Minute)

	for {
		now := time.Now().Format("15:04")
		if app.Config.AutoRebuildTime == now {
			autoRebuildStart(app)
		}
		time.Sleep(time.Minute)
	}
}

func autoRebuildStart(app *App) {
	vmNames := app.VMDB.GetNames()
	for _, vmName := range vmNames {
		err := autoRebuildVM(vmName, app)
		if err != nil {
			app.Log.Errorf("error rebuilding %s: %s", vmName, err)
			app.AlertSender.Send(&Alert{
				Type:    AlertTypeBad,
				Subject: "Auto-rebuild",
				Content: fmt.Sprintf("error rebuilding %s, see server log", vmName.ID()),
			})
		}
	}
}

func autoRebuildVM(vmName *VMName, app *App) error {
	entry, err := app.VMDB.GetEntryByName(vmName)
	if err != nil {
		return err
	}

	// we currently rebuild only active VMs
	if entry.Active == false {
		return nil
	}

	running, _ := VMIsRunning(vmName, app)
	if running == false {
		// VM is down, this is not an error (i guess?)
		return nil
	}

	vm, err := app.VMDB.GetByName(vmName)
	if err != nil {
		return err
	}

	if !IsRebuildNeeded(vm.Config.AutoRebuild, vm.InitDate) {
		return nil
	}

	log := NewLog(vm.Config.Name, app.Hub, app.LogHistory)
	log.Infof("auto-rebuilding %s", vmName)

	operation := app.Operations.Add(&Operation{
		Origin:        "[auto-rebuilder]",
		Action:        "rebuild",
		Ressource:     "vm",
		RessourceName: entry.Name.ID(),
	})
	defer app.Operations.Remove(operation)

	errR := VMRebuild(vmName, false, vm.AuthorKey, app, log)

	// log on VM target
	if errR != nil {
		log.Errorf("auto-rebuild failed for %s", vmName)
	} else {
		log.Infof("auto-rebuild successful for %s", vmName)
	}

	return errR
}

// IsRebuildNeeded return true if lastRebuild is older than rebuildSetting
func IsRebuildNeeded(rebuildSetting string, lastRebuild time.Time) bool {
	lastRebuildAgo := time.Now().Sub(lastRebuild)

	rebuild := false

	if rebuildSetting == VMAutoRebuildDaily && lastRebuildAgo > 24*time.Hour {
		rebuild = true
	}

	if rebuildSetting == VMAutoRebuildWeekly && lastRebuildAgo > 7*24*time.Hour {
		rebuild = true
	}

	if rebuildSetting == VMAutoRebuildMonthly && lastRebuildAgo > 30*24*time.Hour {
		rebuild = true
	}
	return rebuild
}
