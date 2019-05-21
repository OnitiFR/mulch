package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/ssh"
)

// NewVMController creates a new VM
func NewVMController(req *server.Request) {
	configFile, header, err := req.HTTP.FormFile("config")
	if err != nil {
		req.Stream.Failuref("'config' file field: %s", err)
		return
	}
	req.Stream.Tracef("reading '%s' config file", header.Filename)

	restore := req.HTTP.FormValue("restore")
	allowNewRevision := req.HTTP.FormValue("allow_new_revision")

	conf, err := server.NewVMConfigFromTomlReader(configFile, req.Stream)
	if err != nil {
		req.Stream.Failuref("decoding config: %s", err)
		return
	}

	if req.App.VMDB.GetCountForName(conf.Name) > 0 && allowNewRevision != common.TrueStr {
		req.Stream.Failuref("VM '%s' already exists (see --new-revision CLI option?)", conf.Name)
		return
	}

	req.SetTarget(conf.Name)

	if restore != "" {
		conf.RestoreBackup = restore
		req.Stream.Infof("will restore VM from '%s'", restore)
	}

	before := time.Now()
	_, vmName, err := server.NewVM(conf, true, req.APIKey.Comment, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("Cannot create VM: %s", err)
		return
	}

	after := time.Now()

	req.Stream.Successf("VM %s created successfully (%s)", vmName, after.Sub(before))
}

// ListVMsController list VMs
func ListVMsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	vmNames := req.App.VMDB.GetNames()

	var retData common.APIVMListEntries
	for _, vmName := range vmNames {
		vm, err := req.App.VMDB.GetByName(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		domain, err := req.App.Libvirt.GetDomainByName(vmName.LibvirtDomainName(req.App))
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		if domain == nil {
			msg := fmt.Sprintf("VM %s: does not exists in libvirt", vmName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		active, err := req.App.VMDB.IsVMActive(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		// if state == libvirt.DOMAIN_RUNNING {
		// 	// check if services are running? (SSH? port?)
		// }

		retData = append(retData, common.APIVMListEntry{
			Name:      vmName.Name,
			Revision:  vmName.Revision,
			Active:    active,
			LastIP:    vm.LastIP,
			State:     server.LibvirtDomainStateToString(state),
			Locked:    vm.Locked,
			WIP:       string(vm.WIP),
			SuperUser: vm.App.Config.MulchSuperUser,
			AppUser:   vm.Config.AppUser,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		if retData[i].Name == retData[j].Name {
			return retData[i].Revision < retData[j].Revision
		}
		return retData[i].Name < retData[j].Name
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// ActionVMController redirect to the correct action for the VM (start/stop/…)
func ActionVMController(req *server.Request) {
	vmName := req.SubPath

	if vmName == "" {
		req.Stream.Failuref("invalid VM name")
		return
	}
	entry, err := req.App.VMDB.GetActiveEntryByName(vmName)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	vm := entry.VM
	action := req.HTTP.FormValue("action")

	if action != "do" {
		// 'do' actions can send "private" special messages to client (like
		// _MULCH_OPEN_URL) so don't broadcast output to vmName target
		req.SetTarget(vmName)
	}

	switch action {
	case "lock":
		if vm.Locked {
			req.Stream.Warningf("'%s' already locked", vmName)
		}
		err := server.VMLockUnlock(entry.Name, true, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to lock '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("'%s' is now locked", vmName)
		}
	case "unlock":
		if vm.Locked == false {
			req.Stream.Warningf("'%s' already unlocked", vmName)
		}
		err := server.VMLockUnlock(entry.Name, false, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to unlock '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("'%s' is now unlocked", vmName)
		}
	case "start":
		req.Stream.Infof("starting %s", vmName)
		err := server.VMStartByName(entry.Name, vm.SecretUUID, req.App, req.Stream)
		if err != nil {
			req.Stream.Failuref("unable to start '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("VM '%s' is now up and running", vmName)
		}
	case "stop":
		req.Stream.Infof("stopping %s", vmName)
		err := server.VMStopByName(entry.Name, req.App, req.Stream)
		if err != nil {
			req.Stream.Failuref("unable to stop '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("VM '%s' is now down", vmName)
		}
	case "exec":
		err := ExecScriptVM(req, vm, entry.Name)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		}
	case "do":
		err := DoActionVM(req, vm, entry.Name)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		}
	case "backup":
		volHame, err := BackupVM(req, entry.Name)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		} else {
			req.Stream.Successf("backup completed (%s)", volHame)
		}
	case "rebuild":
		before := time.Now()
		err := RebuildVMv2(req, vm, entry.Name)
		after := time.Now()
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		} else {
			req.Stream.Successf("rebuild completed (%s)", after.Sub(before))
		}
	case "redefine":
		err := RedefineVM(req, vm)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		} else {
			req.Stream.Successf("VM '%s' redefined (may the sysadmin gods be with you)", vmName)
		}
	default:
		req.Stream.Failuref("missing or invalid action ('%s') for '%s'", action, vmName)
		return
	}
}

// DeleteVMController will delete a (unlocked) VM
func DeleteVMController(req *server.Request) {
	vmName := req.SubPath
	req.SetTarget(vmName)

	entry, err := req.App.VMDB.GetActiveEntryByName(vmName)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	req.Stream.Infof("deleting vm '%s'", vmName)

	err = server.VMDelete(entry.Name, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("unable to delete VM '%s': %s", vmName, err)
	} else {
		req.Stream.Successf("VM '%s' successfully deleted", vmName)
	}
}

// ExecScriptVM will execute a script inside the VM
func ExecScriptVM(req *server.Request, vm *server.VM, vmName *server.VMName) error {
	script, header, err := req.HTTP.FormFile("script")
	if err != nil {
		return fmt.Errorf("'script' field: %s", err)
	}

	running, _ := server.VMIsRunning(vmName, req.App)
	if running == false {
		return errors.New("VM should be up and running")
	}

	as := req.HTTP.FormValue("as")

	SSHSuperUserAuth, err := req.App.SSHPairDB.GetPublicKeyAuth(server.SSHSuperUserPair)
	if err != nil {
		return err
	}

	run := &server.Run{
		SSHConn: &server.SSHConnection{
			User: vm.App.Config.MulchSuperUser,
			Host: vm.LastIP,
			Port: 22,
			Auths: []ssh.AuthMethod{
				SSHSuperUserAuth,
			},
			Log: req.Stream,
		},
		Tasks: []*server.RunTask{
			&server.RunTask{
				ScriptName:   header.Filename,
				ScriptReader: script,
				As:           as,
			},
		},
		Log: req.Stream,
	}
	err = run.Go()
	if err != nil {
		return err
	}

	req.Stream.Successf("script '%s' returned 0", header.Filename)
	return nil
}

// DoActionVM will execute a "do action" in the VM
func DoActionVM(req *server.Request, vm *server.VM, vmName *server.VMName) error {
	actionName := req.HTTP.FormValue("do_action")
	arguments := req.HTTP.FormValue("arguments")

	action, exists := vm.Config.DoActions[actionName]
	if !exists {
		return fmt.Errorf("unable to find action '%s' for %s", actionName, vmName)
	}

	running, _ := server.VMIsRunning(vmName, req.App)
	if running == false {
		return errors.New("VM should be up and running")
	}

	stream, errG := server.GetScriptFromURL(action.ScriptURL)
	if errG != nil {
		return fmt.Errorf("unable to get script '%s': %s", action.ScriptURL, errG)
	}
	defer stream.Close()

	SSHSuperUserAuth, err := req.App.SSHPairDB.GetPublicKeyAuth(server.SSHSuperUserPair)
	if err != nil {
		return err
	}

	run := &server.Run{
		SSHConn: &server.SSHConnection{
			User: vm.App.Config.MulchSuperUser,
			Host: vm.LastIP,
			Port: 22,
			Auths: []ssh.AuthMethod{
				SSHSuperUserAuth,
			},
			Log: req.Stream,
		},
		Tasks: []*server.RunTask{
			&server.RunTask{
				ScriptName:   path.Base(action.ScriptURL),
				ScriptReader: stream,
				As:           action.User,
				Arguments:    arguments,
			},
		},
		Log: req.Stream,
	}
	err = run.Go()
	if err != nil {
		return err
	}

	req.Stream.Success("script returned 0")
	return nil
}

// GetVMConfigController return a VM config file content
func GetVMConfigController(req *server.Request) {
	vmName := req.SubPath

	if vmName == "" {
		msg := fmt.Sprintf("no VM name given")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}
	vm, err := req.App.VMDB.GetActiveByName(vmName)
	if err != nil {
		msg := fmt.Sprintf("VM '%s' not found", vmName)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	req.Response.Header().Set("Content-Type", "text/plain")
	req.Println(vm.Config.FileContent)
}

// GetVMInfosController return VM informations
func GetVMInfosController(req *server.Request) {
	vmName := req.SubPath

	if vmName == "" {
		msg := fmt.Sprintf("no VM name given")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}
	entry, err := req.App.VMDB.GetActiveEntryByName(vmName)
	if err != nil {
		msg := fmt.Sprintf("VM '%s' not found", vmName)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}
	vm := entry.VM

	running, _ := server.VMIsRunning(entry.Name, req.App)

	diskName, err := server.VMGetDiskName(entry.Name, req.App)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}
	vInfos, err := req.App.Libvirt.VolumeInfos(diskName, req.App.Libvirt.Pools.Disks)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	data := &common.APIVMInfos{
		Name:                entry.Name.Name,
		Revision:            entry.Name.Revision,
		Active:              entry.Active,
		Seed:                vm.Config.Seed,
		CPUCount:            vm.Config.CPUCount,
		RAMSizeMB:           (vm.Config.RAMSize / 1024 / 1024),
		DiskSizeMB:          (vm.Config.DiskSize / 1024 / 1024),
		AllocatedDiskSizeMB: (vInfos.Allocation / 1024 / 1024),
		BackupDiskSizeMB:    (vm.Config.BackupDiskSize / 1024 / 1024),
		Hostname:            vm.Config.Hostname,
		SuperUser:           vm.App.Config.MulchSuperUser,
		AppUser:             vm.Config.AppUser,
		AuthorKey:           vm.AuthorKey,
		InitDate:            vm.InitDate,
		Locked:              vm.Locked,
		Up:                  running,
	}

	req.Response.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(req.Response)
	err = enc.Encode(data)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// GetVMDoActionsController return VM do-action list
func GetVMDoActionsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	vmName := req.SubPath

	if vmName == "" {
		msg := fmt.Sprintf("no VM name given")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}
	vm, err := req.App.VMDB.GetActiveByName(vmName)
	if err != nil {
		msg := fmt.Sprintf("VM '%s' not found", vmName)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	var retData common.APIVMDoListEntries

	for _, action := range vm.Config.DoActions {
		retData = append(retData, common.APIVMDoListEntry{
			Name:        action.Name,
			User:        action.User,
			Description: action.Description,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		return retData[i].Name < retData[j].Name
	})

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// BackupVM launch the backup process
func BackupVM(req *server.Request, vmName *server.VMName) (string, error) {
	return server.VMBackup(vmName, req.App, req.Stream, server.BackupCompressAllow)
}

// RebuildVMv2 delete VM and rebuilds it from a backup (2nd version, using revisions)
func RebuildVMv2(req *server.Request, vm *server.VM, vmName *server.VMName) error {
	if len(vm.Config.Restore) == 0 {
		return errors.New("no restore script defined for this VM")
	}

	if vm.Locked == true {
		return errors.New("VM is locked")
	}

	if vm.WIP != server.VMOperationNone {
		return fmt.Errorf("VM have a work in progress (%s)", string(vm.WIP))
	}

	running, _ := server.VMIsRunning(vmName, req.App)
	if running == false {
		return errors.New("VM should be up and running")
	}

	configFile := vm.Config.FileContent

	conf, err := server.NewVMConfigFromTomlReader(strings.NewReader(configFile), req.Stream)
	if err != nil {
		return fmt.Errorf("decoding config: %s", err)
	}

	conf.RestoreBackup = server.BackupBlankRestore

	success := false

	// create VM rev+1
	// replace original VM author with "rebuilder"
	newVM, newVMName, err := server.NewVM(conf, false, req.APIKey.Comment, req.App, req.Stream)
	if err != nil {
		req.Stream.Error(err.Error())
		return fmt.Errorf("Cannot create VM: %s", err)
	}

	defer func() {
		if success == false {
			err = server.VMDelete(newVMName, req.App, req.Stream)
			if err != nil {
				req.Stream.Error(err.Error())
			}
		}
	}()

	before := time.Now()
	// set rev+0 as inactive ("default" behavior, add a --no-downtime flag?)
	err = req.App.VMDB.SetActiveRevision(vmName.Name, server.RevisionNone)
	if err != nil {
		return fmt.Errorf("can't disable all revisions: %s", err)
	}

	defer func() {
		if success == false {
			err = req.App.VMDB.SetActiveRevision(vmName.Name, vmName.Revision)
			if err != nil {
				req.Stream.Error(err.Error())
			}
		}
	}()

	// backup rev+0
	backupName, err := server.VMBackup(vmName, req.App, req.Stream, server.BackupCompressDisable)
	if err != nil {
		return fmt.Errorf("creating backup: %s", err)
	}

	defer func() {
		// -always- delete backup (success or not)
		err = req.App.BackupsDB.Delete(backupName)
		if err != nil {
			// not a "real" error
			req.Stream.Errorf("unable remove '%s' backup from DB: %s", backupName, err)
		} else {
			err = req.App.Libvirt.DeleteVolume(backupName, req.App.Libvirt.Pools.Backups)
			if err != nil {
				// not a "real" error
				req.Stream.Errorf("unable remove '%s' backup from storage: %s", backupName, err)
			}
		}
	}()

	backup := req.App.BackupsDB.GetByName(backupName)
	if backup == nil {
		return fmt.Errorf("can't find backup '%s' in DB", backupName)
	}

	// restore rev+1
	err = server.VMRestoreNoChecks(newVM, newVMName, backup, req.App, req.Stream)
	if err != nil {
		return fmt.Errorf("restoring backup: %s", err)
	}

	// activate rev+1
	err = req.App.VMDB.SetActiveRevision(newVMName.Name, newVMName.Revision)
	if err != nil {
		return fmt.Errorf("can't enable new revision: %s", err)
	}
	after := time.Now()
	req.Stream.Infof("VM %s is now active", newVMName)

	// - delete rev+0 VM
	err = server.VMDelete(vmName, req.App, req.Stream)
	if err != nil {
		return fmt.Errorf("delete original VM: %s", err)
	}

	// commit (too late to rollback, original VM does not exists anymore)
	success = true

	lock := req.HTTP.FormValue("lock")
	if lock == "true" {
		err := server.VMLockUnlock(newVMName, true, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to lock '%s': %s", vmName, err)
			return nil
		}
		req.Stream.Info("VM locked")
	}

	req.Stream.Infof("downtime: %s", after.Sub(before))

	return nil
}

// RedefineVM replace VM config file with a new one, for next rebuild
func RedefineVM(req *server.Request, vm *server.VM) error {
	if vm.Locked == true {
		return errors.New("VM is locked")
	}

	if vm.WIP != server.VMOperationNone {
		return fmt.Errorf("VM have a work in progress (%s)", string(vm.WIP))
	}

	configFile, header, err := req.HTTP.FormFile("config")
	if err != nil {
		return fmt.Errorf("'config' file field: %s", err)
	}
	req.Stream.Tracef("reading '%s' config file", header.Filename)

	conf, err := server.NewVMConfigFromTomlReader(configFile, req.Stream)
	if err != nil {
		return fmt.Errorf("decoding config: %s", err)
	}

	if conf.Name != vm.Config.Name {
		return fmt.Errorf("VM name does not match")
	}

	// check for conclicting domains
	err = server.CheckDomainsConflicts(req.App.VMDB, conf.Domains, conf.Name)
	if err != nil {
		return err
	}

	// change author
	vm.AuthorKey = req.APIKey.Comment

	oldActions := vm.Config.DoActions

	// redefine config
	vm.Config = conf

	// re-add old 'from prepare' actions (only if a new 'from config' action with
	// the same name is not already defined)
	for name, action := range oldActions {
		if action.FromConfig == true {
			continue
		}
		if _, exists := vm.Config.DoActions[name]; exists {
			req.Stream.Warningf("new action '%s' will replace the old one", name)
			continue
		}
		vm.Config.DoActions[name] = action
	}

	req.App.VMDB.Update()
	if err != nil {
		return err
	}

	return nil
}
