package main

import (
	"net"
	"time"

	"github.com/Xfennec/mulch"
	"golang.org/x/crypto/ssh"
)

// PhoneController receive "phone home" requests from instances
func PhoneController(req *Request) {
	instanceID := req.HTTP.PostFormValue("instance_id")
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)

	// Cloud-Init sends fqdn, hostname and SSH pub keys, but our "manual"
	// call does not. It's an easy way to know who called us.
	cloudInit := false
	if req.HTTP.PostFormValue("fqdn") != "" {
		cloudInit = true
	}

	if instanceID == "" {
		req.App.Log.Errorf("invalid phone call from %s (no or empty instance_id)", ip)
		req.Response.Write([]byte("FAILED"))
		return
	}

	instanceAnon := "?"
	if len(instanceID) > 4 {
		instanceAnon = instanceID[:4] + "…"
	}

	// We should lookup the machine and log over there, no?
	req.App.Log.Infof("phoning: id=%s, ip=%s", instanceAnon, ip)
	for key, val := range req.HTTP.Form {
		if key == "instance_id" {
			val[0] = instanceAnon
		}
		req.App.Log.Tracef(" - %s = '%s'", key, val[0])
	}

	req.App.PhoneHome.BroadcastPhoneCall(instanceID, ip, cloudInit)
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
		Name:        "test1",
		Hostname:    "test1.localdomain",
		SeedImage:   "debian-9-openstack-amd64.qcow2",
		InitUpgrade: false,
		DiskSize:    50 * 1024 * 1024 * 1024,
		RAMSize:     2 * 1024 * 1024 * 1024,
		CPUCount:    2,
	}

	// TODO: check the name before doing that:
	// No other libvirt VM with this name in our database?
	// Name is valid?
	req.SetTarget(conf.Name)

	before := time.Now()
	vm, err := NewVM(conf, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("Cannot create VM: %s", err)
		return
	}
	after := time.Now()

	req.Stream.Successf("VM '%s' created successfully (%s)", vm.Config.Name, after.Sub(before))
}

// TestController is a test. Yep.
func TestController(req *Request) {
	run := &Run{
		SSHConn: &SSHConnection{
			User: req.App.Config.MulchSuperUser,
			Host: "10.104.24.93",
			Port: 22,
			Auths: []ssh.AuthMethod{
				PublicKeyFile(req.App.Config.MulchSSHPrivateKey),
			},
			Log: req.Stream,
		},
		Tasks: []*RunTask{
			&RunTask{Script: "a1.sh"},
			&RunTask{Script: "a2.sh"},
		},
		Log: req.Stream,
	}
	err := run.Go()
	if err != nil {
		req.Stream.Error(err.Error())
	}
	req.Stream.Info("exit")
}
