package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	conf, err := server.NewVMConfigFromTomlReader(configFile, req.Stream)
	if err != nil {
		req.Stream.Failuref("decoding config: %s", err)
		return
	}

	req.SetTarget(conf.Name)

	if restore != "" {
		conf.RestoreBackup = restore
		req.Stream.Infof("will restore VM from '%s'", restore)
	}

	before := time.Now()
	vm, err := server.NewVM(conf, req.APIKey.Comment, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("Cannot create VM: %s", err)
		return
	}
	after := time.Now()

	req.Stream.Successf("VM '%s' created successfully (%s)", vm.Config.Name, after.Sub(before))
}

// ListVMsController list VMs
func ListVMsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	vmNames := req.App.VMDB.GetNames()

	var retData common.APIVMListEntries
	for _, vmName := range vmNames {
		vm, err := req.App.VMDB.GetByName(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		domain, err := req.App.Libvirt.GetDomainByName(req.App.Config.VMPrefix + vmName)
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		if domain == nil {
			msg := fmt.Sprintf("VM '%s': does not exists in libvirt", vmName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		// if state == libvirt.DOMAIN_RUNNING {
		// 	// check if services are running? (SSH? port?)
		// }

		retData = append(retData, common.APIVMListEntry{
			Name:      vmName,
			LastIP:    vm.LastIP,
			State:     server.LibvirtDomainStateToString(state),
			Locked:    vm.Locked,
			WIP:       string(vm.WIP),
			SuperUser: vm.App.Config.MulchSuperUser,
			AppUser:   vm.Config.AppUser,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
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
	vm, err := req.App.VMDB.GetByName(vmName)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	req.SetTarget(vmName)

	libvirtVMName := vm.App.Config.VMPrefix + vmName

	action := req.HTTP.FormValue("action")
	switch action {
	case "lock":
		err := server.VMLockUnlock(vmName, true, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to lock '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("'%s' is now locked", vmName)
		}
	case "unlock":
		err := server.VMLockUnlock(vmName, false, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to unlock '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("'%s' is now unlocked", vmName)
		}
	case "start":
		req.Stream.Infof("starting %s", vmName)
		err := server.VMStartByName(libvirtVMName, vm.SecretUUID, req.App, req.Stream)
		if err != nil {
			req.Stream.Failuref("unable to start '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("VM '%s' is now up and running", vmName)
		}
	case "stop":
		req.Stream.Infof("stopping %s", vmName)
		err := server.VMStopByName(libvirtVMName, req.App, req.Stream)
		if err != nil {
			req.Stream.Failuref("unable to stop '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("VM '%s' is now down", vmName)
		}
	case "exec":
		// req.Stream.Infof("executing script (%s)", vmName)
		err := ExecScriptVM(req, vm)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		}
	case "backup":
		volHame, err := BackupVM(req, vm)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
		} else {
			req.Stream.Successf("backup completed (%s)", volHame)
		}
	case "rebuild":
		before := time.Now()
		err := RebuildVM(req, vm)
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
			req.Stream.Successf("VM '%s' redefined (may the sysadmin gods be with you)", vm.Config.Name)
		}
	default:
		req.Stream.Failuref("missing or invalid action ('%s') for '%s'", action, vm.Config.Name)
		return
	}
}

// DeleteVMController will delete a (unlocked) VM
func DeleteVMController(req *server.Request) {
	vmName := req.SubPath
	req.SetTarget(vmName)
	req.Stream.Infof("deleting vm '%s'", vmName)
	err := server.VMDelete(vmName, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("unable to delete VM '%s': %s", vmName, err)
	} else {
		req.Stream.Successf("VM '%s' successfully deleted", vmName)
	}
}

// ExecScriptVM will execute a script inside the VM
func ExecScriptVM(req *server.Request, vm *server.VM) error {
	script, header, err := req.HTTP.FormFile("script")
	if err != nil {
		return fmt.Errorf("'script' field: %s", err)
	}
	// TODO: check shebang? (and then rewind the reader ?)

	// Some sort of "security" check (even if we're root on the VM…)
	as := req.HTTP.FormValue("as")
	if !server.IsValidName(as) {
		return fmt.Errorf("'%s' is not a valid username", as)
	}

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

// GetVMConfigController return a VM config file content
func GetVMConfigController(req *server.Request) {
	vmName := req.SubPath

	if vmName == "" {
		msg := fmt.Sprintf("no VM name given")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}
	vm, err := req.App.VMDB.GetByName(vmName)
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
	vm, err := req.App.VMDB.GetByName(vmName)
	if err != nil {
		msg := fmt.Sprintf("VM '%s' not found", vmName)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	running, _ := server.VMIsRunning(vm.Config.Name, req.App)

	libvirtName := req.App.Config.VMPrefix + vmName

	diskName, err := server.VMGetDiskName(libvirtName, req.App)
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
		Name:                vm.Config.Name,
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
	vm, err := req.App.VMDB.GetByName(vmName)
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

// BackupVM launch the backup proccess
func BackupVM(req *server.Request, vm *server.VM) (string, error) {
	return server.VMBackup(vm.Config.Name, req.App, req.Stream, server.BackupCompressAllow)
}

// RebuildVM delete VM and rebuilds it from a backup
// steps: backup, stop, rename-old, create-with-restore, delete-old, delete backup
func RebuildVM(req *server.Request, vm *server.VM) error {
	if len(vm.Config.Restore) == 0 {
		return errors.New("no restore script defined for this VM")
	}

	if vm.Locked == true {
		return errors.New("VM is locked")
	}

	configFile := vm.Config.FileContent

	vmName := vm.Config.Name
	libvirtVMName := vm.App.Config.VMPrefix + vmName

	// - backup
	backupName, err := server.VMBackup(vmName, req.App, req.Stream, server.BackupCompressDisable)
	if err != nil {
		return fmt.Errorf("creating backup: %s", err)
	}

	// - stop
	req.Stream.Infof("stopping VM")
	err = server.VMStopByName(libvirtVMName, req.App, req.Stream)
	if err != nil {
		return err
	}

	// - rename original VM
	req.Stream.Infof("cloning VM")
	tmpVMName := fmt.Sprintf("%s-old-%d", vmName, req.App.Rand.Int31())
	err = server.VMRename(vmName, tmpVMName, req.App, req.Stream)
	if err != nil {
		return err
	}

	// remove domains from previous VM so the new one can take its place
	domains := vm.Config.Domains
	vm.Config.Domains = nil

	success := false
	defer func() {
		if success == false {
			req.Stream.Infof("rollback: re-creating VM from %s", tmpVMName)
			// get our domains backs
			vm.Config.Domains = domains
			err = server.VMRename(tmpVMName, vmName, req.App, req.Stream)
			if err != nil {
				req.Stream.Error(err.Error())
				return
			}
			err = server.VMStartByName(libvirtVMName, vm.SecretUUID, req.App, req.Stream)
			if err != nil {
				req.Stream.Error(err.Error())
				return
			}
			req.Stream.Info("original VM restored")
		}
	}()

	// - re-create VM
	conf, err := server.NewVMConfigFromTomlReader(strings.NewReader(configFile), req.Stream)
	if err != nil {
		return fmt.Errorf("decoding config: %s", err)
	}

	conf.RestoreBackup = backupName

	// replace original VM author with "rebuilder"
	_, err = server.NewVM(conf, req.APIKey.Comment, req.App, req.Stream)
	if err != nil {
		req.Stream.Error(err.Error())
		return fmt.Errorf("Cannot create VM: %s", err)
	}

	// - delete backup
	err = req.App.BackupsDB.Delete(backupName)
	if err != nil {
		req.Stream.Errorf("unable remove '%s' backup from DB: %s", backupName, err)
		return nil // not a real error
	}
	err = req.App.Libvirt.DeleteVolume(backupName, req.App.Libvirt.Pools.Backups)
	if err != nil {
		req.Stream.Errorf("unable remove '%s' backup from storage: %s", backupName, err)
		return nil // not a real error
	}

	// - delete original VM
	err = server.VMDelete(tmpVMName, req.App, req.Stream)
	if err != nil {
		return fmt.Errorf("delete VM: %s", err)
	}

	lock := req.HTTP.FormValue("lock")
	if lock == "true" {
		err := server.VMLockUnlock(vmName, true, req.App.VMDB)
		if err != nil {
			req.Stream.Failuref("unable to lock '%s': %s", vmName, err)
			return nil
		}
		req.Stream.Info("VM locked")
	}

	// commit
	success = true

	return nil
}

// RedefineVM replace VM config file with a new one, for next rebuild
func RedefineVM(req *server.Request, vm *server.VM) error {
	req.SetTarget(vm.Config.Name)

	if vm.Locked == true {
		return errors.New("VM is locked")
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

	vm.Config = conf

	req.App.VMDB.Update()
	if err != nil {
		return err
	}

	return nil
}
