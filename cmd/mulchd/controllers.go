package main

import (
	"fmt"
	"net"
	"strconv"
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

	vm, err := req.App.VMDB.GetBySecretUUID(instanceID)
	if err != nil {
		if cloudInit == false {
			req.App.Log.Warningf("no VM found (yet?) in database with this instance_id (%s)", instanceAnon)
		}
	} else {
		req.App.Log.Infof("phoning VM is '%s'", vm.Config.Name)
		if vm.LastIP != ip {
			req.App.Log.Warningf("vm IP changed since last call (from '%s' to '%s')", vm.LastIP, ip)

			vm.LastIP = ip
			err = req.App.VMDB.Update()
			if err != nil {
				req.App.Log.Errorf("unable to update VM DB: %s", err)
			}
		}
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

	configFile, header, err := req.HTTP.FormFile("config")
	if err != nil {
		req.Stream.Failuref("'config' file field: %s", err)
		return
	}
	req.Stream.Tracef("reading '%s' config file", header.Filename)

	conf, err := NewVMConfigFromTomlReader(configFile)
	if err != nil {
		req.Stream.Failuref("decoding config: %s", err)
		return
	}

	before := time.Now()
	vm, err := NewVM(conf, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("Cannot create VM: %s", err)
		return
	}
	after := time.Now()

	req.Stream.Successf("VM '%s' created successfully (%s)", vm.Config.Name, after.Sub(before))
}

// VersionController return versions
func VersionController(req *Request) {
	req.Response.Header().Set("Content-Type", "text/plain")
	req.Response.Write([]byte(fmt.Sprintf("server version: %s\n", Version)))
	req.Response.Write([]byte(fmt.Sprintf("server protocol: %s\n", strconv.Itoa(ProtocolVersion))))
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

// Test2Controller is another test.
func Test2Controller(req *Request) {
	req.Stream.Trace("test trace")
	req.Stream.Infof("your version: %s", req.HTTP.PostFormValue("version"))
	req.Stream.Info("test info")
	req.Stream.Warning("test warning")
	req.Stream.Error("test warning")

	req.Stream.Success("test success")
	req.Stream.Failure("test failure")
}
