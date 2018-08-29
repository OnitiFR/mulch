package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/olekukonko/tablewriter"
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
	req.Response.Header().Set("Content-Type", "text/plain")
	vmNames := req.App.VMDB.GetNames()

	if len(vmNames) == 0 {
		req.Printf("Currently, no VM exists. You may use 'mulch vm create'.\n")
		return
	}

	tableData := [][]string{}
	for _, vmName := range vmNames {
		vm, err := req.App.VMDB.GetByName(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM '%s': %s\n", vmName, err)
			req.App.Log.Error(msg)
			req.Println(msg)
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

		tableData = append(tableData, []string{
			vmName,
			vm.LastIP,
			server.LibvirtDomainStateToString(state),
			strconv.FormatBool(vm.Locked),
		})
	}

	table := tablewriter.NewWriter(req.Response)
	table.SetHeader([]string{"Name", "Last known IP", "State", "Locked"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(tableData)
	table.Render()
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
		req.Stream.Infof("stoping %s", vmName)
		err := server.VMStopByName(libvirtVMName, req.App, req.Stream)
		if err != nil {
			req.Stream.Failuref("unable to stop '%s': %s", vmName, err)
		} else {
			req.Stream.Successf("VM '%s' is now down", vmName)
		}
	default:
		req.Stream.Failuref("missing or invalid action ('%s') for '%s'", action, vm.Config.Name)
		return
	}
}
