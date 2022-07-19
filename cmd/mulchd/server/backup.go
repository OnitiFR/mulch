package server

import (
	"fmt"
	"time"

	libvirt "gopkg.in/libvirt/libvirt-go.v7"
)

// Backup describes a VM backup
type Backup struct {
	DiskName  string
	Created   time.Time
	Expire    time.Time
	AuthorKey string
	VM        *VM
}

func BackupDelete(backupName string, app *App) error {
	backup := app.BackupsDB.GetByName(backupName)
	if backup == nil {
		return fmt.Errorf("backup '%s' not found in database", backupName)
	}

	vol, errDef := app.Libvirt.Pools.Backups.LookupStorageVolByName(backupName)
	if errDef != nil {
		return fmt.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, backupName)
	}
	defer vol.Free()
	errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if errDef != nil {
		return fmt.Errorf("failed Delete: %s (%s)", errDef, backupName)
	}

	err := app.BackupsDB.Delete(backupName)
	if err != nil {
		return fmt.Errorf("unable remove '%s' backup from DB: %s", backupName, err)
	}
	return nil
}
