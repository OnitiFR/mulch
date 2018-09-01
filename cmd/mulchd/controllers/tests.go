package controllers

import (
	"golang.org/x/crypto/ssh"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
)

// TestController is a test. Yep.
func TestController(req *server.Request) {

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
				Script: "a1.sh",
				As:     "admin",
			},
			&server.RunTask{
				Script: "a2.sh",
				As:     "app",
			},
			&server.RunTask{
				Script: "a1.sh",
				As:     "admin",
			},
			&server.RunTask{
				Script: "a2.sh",
				As:     "app",
			},
		},
		Log: req.Stream,
	}
	err = run.Go()
	if err != nil {
		req.Stream.Error(err.Error())
	}
	req.Stream.Info("exit")
}

// Test2Controller is another test.
func Test2Controller(req *server.Request) {
	req.Stream.Trace("test trace")
	req.Stream.Infof("your version: %s", req.HTTP.PostFormValue("version"))
	req.Stream.Info("test info")
	req.Stream.Warning("test warning")
	req.Stream.Error("test warning")

	req.Stream.Success("test success")
	req.Stream.Failure("test failure")
}
