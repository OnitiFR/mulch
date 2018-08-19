package main

import (
	"fmt"
	"net"

	"github.com/Xfennec/mulch"
)

// PhoneController receive "phone home" requests from instances
func PhoneController(req *Request) {
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	msg := fmt.Sprintf("phoning: id=%s, ip=%s", req.HTTP.PostFormValue("instance_id"), ip)

	// fmt.Println(req.HTTP)
	for key, val := range req.HTTP.Form {
		req.App.Log.Tracef(" - %s = '%s'", key, val[0])
	}
	// We should lookup the machine and log over there, no?
	req.App.Log.Info(msg)

	req.Response.Write([]byte("OK"))
}

// LogController sends logs to client
func LogController(req *Request) {
	req.Stream.Infof("Hi! You will receive all logs for all targets.")
	req.SetTarget(mulch.MessageAllTargets)
	// nothing to do, just wait forever…
	select {}
}

// VMController is currently a test
func VMController(req *Request) {
	conf := &VMConfig{
		Name:      "test1",
		Hostname:  "test1.localdomain",
		SeedImage: "debian-9-openstack-amd64.qcow2",
		DiskSize:  20 * 1024 * 1024 * 1024,
		RAMSize:   1 * 1024 * 1024 * 1024,
		CPUCount:  1,
	}

	// TODO: check the name before doing that:
	// No other libvirt VM with this name in our database?
	// Name is valid?
	req.SetTarget(conf.Name)

	vm, err := NewVM(conf, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("Cannot create VM: %s", err)
		return
	}

	req.Stream.Successf("VM '%s' created successfully (%s)", vm.Config.Name, vm.UUID)
}
