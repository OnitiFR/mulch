package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
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

	conf, err := server.NewVMConfigFromTomlReader(configFile)
	if err != nil {
		req.Stream.Failuref("decoding config: %s", err)
		return
	}

	before := time.Now()
	vm, err := server.NewVM(conf, req.App, req.Stream)
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
			msg := fmt.Sprintf("VM '%s': %s\n", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		domain, err := req.App.Libvirt.GetDomainByName(req.App.Config.VMPrefix + vmName)
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s\n", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		if domain == nil {
			msg := fmt.Sprintf("VM '%s': does not exists in libvirt\n", vmName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s\n", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		// if state == libvirt.DOMAIN_RUNNING {
		// 	// check if services are running? (SSH? port?)
		// }

		retData = append(retData, common.APIVmListEntry{
			Name:   vmName,
			LastIP: vm.LastIP,
			State:  server.LibvirtDomainStateToString(state),
			Locked: vm.Locked,
		})
	}

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// ActionVMController redirect to the correct action for the VM (start/stop)
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
		err := ExecScriptVM(req)
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
	req.Stream.Infof("deleting %s", vmName)
	err := server.VMDelete(vmName, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("unable to delete '%s': %s", vmName, err)
	} else {
		req.Stream.Successf("'%s' successfully deleted", vmName)
	}
}

// ExecScriptVM will execute a script inside the VM
func ExecScriptVM(req *server.Request) error {
	vmName := req.SubPath

	if vmName == "" {
		return errors.New("invalid VM name")
	}
	vm, err := req.App.VMDB.GetByName(vmName)
	if err != nil {
		return err
	}

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

	run := &server.Run{
		SSHConn: &server.SSHConnection{
			User: vm.App.Config.MulchSuperUser,
			Host: vm.LastIP,
			Port: 22,
			Auths: []ssh.AuthMethod{
				server.PublicKeyFile(vm.App.Config.MulchSSHPrivateKey),
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
