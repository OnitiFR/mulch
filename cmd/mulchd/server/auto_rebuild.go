package server

import (
	"fmt"
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

	lastRebuild := time.Now().Sub(vm.InitDate)
	rebuild := false

	if vm.Config.AutoRebuild == VMAutoRebuildDaily && lastRebuild > 24*time.Hour {
		rebuild = true
	}

	if vm.Config.AutoRebuild == VMAutoRebuildWeekly && lastRebuild > 7*24*time.Hour {
		rebuild = true
	}

	if vm.Config.AutoRebuild == VMAutoRebuildMonthly && lastRebuild > 30*24*time.Hour {
		rebuild = true
	}

	if !rebuild {
		return nil
	}

	app.Log.Infof("auto-rebuilding %s", vmName)

	operation := app.Operations.Add(&Operation{
		Origin:        "[auto-rebuilder]",
		Action:        "rebuild",
		Ressource:     "vm",
		RessourceName: entry.Name.ID(),
	})
	defer app.Operations.Remove(operation)

	return VMRebuild(vmName, false, vm.AuthorKey, app, app.Log)
}
