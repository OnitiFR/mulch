package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
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

		retData = append(retData, common.APIBackupListEntry{
			DiskName: backup.DiskName,
			VMName:   backup.VM.Config.Name,
			Created:  backup.Created,
		})
	}

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
