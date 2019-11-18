package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/cmd/mulchd/volumes"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"gopkg.in/libvirt/libvirt-go.v5"
)

// ListBackupsController list Backups
func ListBackupsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	backupNames := req.App.BackupsDB.GetNames()

	vmFilter := req.HTTP.FormValue("vm")

	if vmFilter != "" {
		if req.App.VMDB.GetCountForName(vmFilter) == 0 {
			msg := fmt.Sprintf("can't find any VM with name '%s'", vmFilter)
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
			AuthorKey: backup.AuthorKey,
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
	req.StartStream <- true
	backupName := req.SubPath
	req.Stream.Infof("deleting backup '%s'", backupName)

	operation := req.App.Operations.Add(&server.Operation{
		Origin:        req.APIKey.Comment,
		Action:        "delete",
		Ressource:     "backup",
		RessourceName: backupName,
	})
	defer req.App.Operations.Remove(operation)

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
		return
	}

	req.Stream.Successf("backup '%s' successfully deleted", backupName)
}

// DownloadBackupController will download a backup image
func DownloadBackupController(req *server.Request) {
	backupName := req.SubPath

	operation := req.App.Operations.Add(&server.Operation{
		Origin:        req.APIKey.Comment,
		Action:        "download",
		Ressource:     "backup",
		RessourceName: backupName,
	})
	defer req.App.Operations.Remove(operation)

	req.Response.Header().Set("Content-Type", "application/octet-stream")

	conn, err := req.App.Libvirt.GetConnection()
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	backup := req.App.BackupsDB.GetByName(backupName)
	if backup == nil {
		errB := fmt.Errorf("backup '%s' not found in database", backupName)
		req.App.Log.Error(errB.Error())
		http.Error(req.Response, errB.Error(), 500)
		return
	}

	vol, err := req.App.Libvirt.Pools.Backups.LookupStorageVolByName(backupName)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}
	defer vol.Free()

	writeCloser := &common.FakeWriteCloser{Writer: req.Response}
	vd, err := volumes.NewVolumeDownloadToWriter(vol, conn, writeCloser)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	bytesWritten, err := vd.Copy()
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}
	req.App.Log.Tracef("client downloaded %s (%s)", backupName, (datasize.ByteSize(bytesWritten) * datasize.B).HR())
}

// UploadBackupController will upload a backup image to storage
func UploadBackupController(req *server.Request) {
	req.StartStream <- true
	file, header, err := req.HTTP.FormFile("file")
	if err != nil {
		req.Stream.Failuref("error with 'file' field: %s", err)
		return
	}

	if req.App.BackupsDB.GetByName(header.Filename) != nil {
		req.Stream.Failuref("backup '%s' already exists in database", header.Filename)
		return
	}

	operation := req.App.Operations.Add(&server.Operation{
		Origin:        req.APIKey.Comment,
		Action:        "upload",
		Ressource:     "backup",
		RessourceName: header.Filename,
	})
	defer req.App.Operations.Remove(operation)

	req.Stream.Infof("uploading '%s'", header.Filename)

	err = req.App.Libvirt.UploadFileToLibvirtFromReader(
		req.App.Libvirt.Pools.Backups,
		req.App.Libvirt.Pools.BackupsXML,
		req.App.Config.GetTemplateFilepath("volume.xml"),
		file,
		header.Filename,
		req.Stream)

	if err != nil {
		req.Stream.Failuref("unable to upload backup: %s", err)
		return
	}

	// Create a backup in DB with an empty VM
	backup := &server.Backup{
		DiskName:  header.Filename,
		Created:   time.Now(),
		AuthorKey: req.APIKey.Comment,
		VM: &server.VM{
			Config: &server.VMConfig{},
		},
	}

	err = req.App.BackupsDB.Add(backup)
	if err != nil {
		req.Stream.Failuref("error adding backup to DB: %s", err)
		return
	}

	req.Stream.Successf("backup '%s' uploaded successfully", header.Filename)
}
