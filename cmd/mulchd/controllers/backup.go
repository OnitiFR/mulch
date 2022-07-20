package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/cmd/mulchd/volumes"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
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
			Expire:    backup.Expire,
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

func deleteBackup(backupName string, req *server.Request) error {
	return server.BackupDelete(backupName, req.App)
}

// DeleteBackupController will delete a backup
func DeleteBackupController(req *server.Request) {
	req.StartStream()
	backupName := req.SubPath
	req.Stream.Infof("deleting backup '%s'", backupName)

	operation := req.App.Operations.Add(&server.Operation{
		Origin:        req.APIKey.Comment,
		Action:        "delete",
		Ressource:     "backup",
		RessourceName: backupName,
	})
	defer req.App.Operations.Remove(operation)

	err := deleteBackup(backupName, req)
	if err != nil {
		req.Stream.Failure(err.Error())
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
	req.StartStream()
	file, header, err := req.HTTP.FormFile("file")
	if err != nil {
		req.Stream.Failuref("error with 'file' field: %s (not enough temp space?)", err)
		return
	}

	expireStr := req.HTTP.FormValue("expire")
	expire := time.Duration(0)
	if expireStr != "" {
		seconds, err := strconv.Atoi(expireStr)
		if err != nil {
			req.Stream.Failuref("unable to parse expire value")
			return

		}
		expire = time.Duration(seconds) * time.Second
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

	if expire > server.BackupNoExpiration {
		backup.Expire = time.Now().Add(expire)
	}

	err = req.App.BackupsDB.Add(backup)
	if err != nil {
		req.Stream.Failuref("error adding backup to DB: %s", err)
		return
	}

	req.Stream.Successf("backup '%s' uploaded successfully", header.Filename)
}

func SetBackupExpireController(req *server.Request) {
	req.StartStream()

	backupName := req.SubPath

	expireStr := req.HTTP.FormValue("expire")
	expire := time.Duration(0)
	if expireStr != "" {
		seconds, err := strconv.Atoi(expireStr)
		if err != nil {
			req.Stream.Failuref("unable to parse expire value")
			return

		}
		expire = time.Duration(seconds) * time.Second
	}

	expireDate := time.Time{}
	if expire > server.BackupNoExpiration {
		expireDate = time.Now().Add(expire)
	}

	err := req.App.BackupsDB.Expire(backupName, expireDate)
	if err != nil {
		req.Stream.Failuref("unable to set expire value: %s", err)
		return
	}

	if expire > server.BackupNoExpiration {
		req.Stream.Successf("backup will expire in %s (%s)", expire, expireDate.Format("2006-01-02 15:04"))
	} else {
		req.Stream.Successf("backup '%s' will never expire", backupName)
	}
}
