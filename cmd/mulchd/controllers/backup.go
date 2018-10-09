package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
	"github.com/libvirt/libvirt-go"
)

// ListBackupsController list Backups
func ListBackupsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	backupNames := req.App.BackupsDB.GetNames()

	vmFilter := req.HTTP.FormValue("vm")

	if vmFilter != "" {
		_, err := req.App.VMDB.GetByName(vmFilter)
		if err != nil {
			msg := fmt.Sprintf("'%s': %s", vmFilter, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 404)
			return
		}
	}

	var retData common.APIBackupListEntries
	for _, backupName := range backupNames {
		backup := req.App.BackupsDB.GetByName(backupName)
		if backup == nil {
			msg := fmt.Sprintf("backup '%s' not found", backupName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		if vmFilter != "" && vmFilter != backup.VM.Config.Name {
			continue
		}

		infos, err := req.App.Libvirt.VolumeInfos(backupName, req.App.Libvirt.Pools.Backups)
		if err != nil {
			req.App.Log.Error(err.Error())
			http.Error(req.Response, err.Error(), 500)
			return
		}

		retData = append(retData, common.APIBackupListEntry{
			DiskName:  backup.DiskName,
			VMName:    backup.VM.Config.Name,
			Created:   backup.Created,
			Size:      infos.Capacity,
			AllocSize: infos.Allocation,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		return retData[i].Created.Before(retData[j].Created)
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// DeleteBackupController will delete a backup
func DeleteBackupController(req *server.Request) {
	backupName := req.SubPath
	req.Stream.Infof("deleting backup '%s'", backupName)

	backup := req.App.BackupsDB.GetByName(backupName)
	if backup == nil {
		req.Stream.Failuref("backup '%s' not found in database", backupName)
		return
	}

	vol, errDef := req.App.Libvirt.Pools.Backups.LookupStorageVolByName(backupName)
	if errDef != nil {
		req.Stream.Failuref("failed LookupStorageVolByName: %s (%s)", errDef, backupName)
		return
	}
	defer vol.Free()
	errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if errDef != nil {
		req.Stream.Failuref("failed Delete: %s (%s)", errDef, backupName)
		return
	}

	err := req.App.BackupsDB.Delete(backupName)
	if err != nil {
		req.Stream.Failuref("unable remove '%s' backup from DB: %s", backupName, err)
	}

	req.Stream.Successf("backup '%s' successfully deleted", backupName)
}
