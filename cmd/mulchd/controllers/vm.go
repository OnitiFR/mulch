package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"time"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
	"github.com/libvirt/libvirt-go"
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

	conf, err := server.NewVMConfigFromTomlReader(configFile, req.Stream)
	if err != nil {
		req.Stream.Failuref("decoding config: %s", err)
		return
	}

	req.SetTarget(conf.Name)
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

	var retData common.APIVmListEntries
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

		retData = append(retData, common.APIVmListEntry{
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
		err := BackupVM(req, vm)
		if err != nil {
			req.Stream.Failuref("error: %s", err)
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
	if !server.IsValidTokenName(as) {
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

	data := &common.APIVmInfos{
		Name:      vm.Config.Name,
		Seed:      vm.Config.Seed,
		CPUCount:  vm.Config.CPUCount,
		RAMSize:   vm.Config.RAMSize,
		Hostname:  vm.Config.Hostname,
		SuperUser: vm.App.Config.MulchSuperUser,
		AppUser:   vm.Config.AppUser,
		AuthorKey: vm.AuthorKey,
		Locked:    vm.Locked,
		Up:        running,
	}

	req.Response.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(req.Response)
	err = enc.Encode(data)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// BackupVM launch the backup proccess
func BackupVM(req *server.Request, vm *server.VM) error {

	if vm.WIP != server.VMOperationNone {
		return fmt.Errorf("VM already have a work in progress (%s)", string(vm.WIP))
	}

	vm.SetOperation(server.VMOperationBackup)
	defer vm.SetOperation(server.VMOperationNone)

	running, _ := server.VMIsRunning(vm.Config.Name, req.App)
	if running == false {
		return errors.New("VM should be up and running to do a backup")
	}

	if len(vm.Config.Backup) == 0 {
		return errors.New("no backup script defined for this VM")
	}

	volName := fmt.Sprintf("%s-backup-%s.qcow2",
		vm.Config.Name,
		time.Now().Format("20060102-150405"),
	)

	if req.App.BackupsDB.GetByName(volName) != nil {
		return fmt.Errorf("a backup with the same name already exists (%s)", volName)
	}

	SSHSuperUserAuth, err := req.App.SSHPairDB.GetPublicKeyAuth(server.SSHSuperUserPair)
	if err != nil {
		return err
	}

	err = server.VMCreateBackupDisk(vm.Config.Name, volName, vm.Config.BackupDiskSize, req.App, req.Stream)
	if err != nil {
		return err
	}

	// NOTE: this attachement is transient
	err = server.VMAttachBackup(vm.Config.Name, volName, req.App)
	if err != nil {
		return err
	}
	req.Stream.Info("backup disk attached")

	// defer detach + vol delete in case of failure
	commit := false
	defer func() {
		if commit == false {
			req.Stream.Info("rollback backup disk creation")
			errDet := server.VMDetachBackup(vm.Config.Name, req.App)
			if errDet != nil {
				req.Stream.Errorf("failed trying VMDetachBackup: %s (%s)", errDet, volName)
				// no return, it may be already detached
			}
			vol, errDef := req.App.Libvirt.Pools.Backups.LookupStorageVolByName(volName)
			if errDef != nil {
				req.Stream.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, volName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				req.Stream.Errorf("failed Delete: %s (%s)", errDef, volName)
				return
			}
		}
	}()

	pre, err := os.Open(req.App.Config.GetTemplateFilepath("pre-backup.sh"))
	if err != nil {
		return err
	}
	defer pre.Close()

	post, err := os.Open(req.App.Config.GetTemplateFilepath("post-backup.sh"))
	if err != nil {
		return err
	}
	defer post.Close()

	before := time.Now()

	// pre-backup + backup + post-backup
	tasks := []*server.RunTask{}
	tasks = append(tasks, &server.RunTask{
		ScriptName:   "pre-backup.sh",
		ScriptReader: pre,
		As:           vm.App.Config.MulchSuperUser,
	})

	for _, confTask := range vm.Config.Backup {
		stream, errG := server.GetScriptFromURL(confTask.ScriptURL)
		if errG != nil {
			return fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
		}
		defer stream.Close()

		task := &server.RunTask{
			ScriptName:   path.Base(confTask.ScriptURL),
			ScriptReader: stream,
			As:           confTask.As,
		}
		tasks = append(tasks, task)
	}

	tasks = append(tasks, &server.RunTask{
		ScriptName:   "post-backup.sh",
		ScriptReader: post,
		As:           vm.App.Config.MulchSuperUser,
	})

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
		Tasks: tasks,
		Log:   req.Stream,
	}
	err = run.Go()
	if err != nil {
		return err
	}

	// detach backup disk
	// TODO: check if this operation is synchronous for QEMU!
	err = server.VMDetachBackup(vm.Config.Name, req.App)
	if err != nil {
		return err
	}
	req.Stream.Info("backup disk detached")

	err = req.App.Libvirt.BackupCompress(volName, req.App.Config.GetTemplateFilepath("volume.xml"), req.Stream)
	if err != nil {
		return err
	}

	req.App.BackupsDB.Add(&server.Backup{
		DiskName: volName,
		Created:  time.Now(),
		VM:       vm,
	})
	after := time.Now()

	req.Stream.Successf("backup completed (%s)", after.Sub(before))
	commit = true
	return nil
}
