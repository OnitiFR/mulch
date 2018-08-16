package main

import (
	"fmt"
	"net"
	"time"
)

// PhoneController receive "phone home" requests from instances
func PhoneController(req *Request) {
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	msg := fmt.Sprintf("phoning: id=%s, ip=%s", req.HTTP.PostFormValue("instance_id"), ip)

	// We should lookup the machine and log over there, no?
	req.App.Log.Info(msg)

	req.Response.Write([]byte("OK"))
}

// LogController sends logs to client
func LogController(req *Request) {
	req.SetTarget("instance-1")
	req.Stream.Info("Hello from LogController")
	time.Sleep(time.Duration(5000) * time.Millisecond)
	req.Stream.Info("Bye from LogController")
}

// VMController is currently a test
func VMController(req *Request) {
	conf := &VMConfig{
		Name:           "test1",
		ReferenceImage: "debian-9-openstack-amd64.qcow2",
		DiskSize:       20 * 1024 * 1024 * 1024,
		RAMSize:        1 * 1024 * 1024 * 1024,
		CPUCount:       1,
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
