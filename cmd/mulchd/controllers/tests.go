package controllers

import (
	"golang.org/x/crypto/ssh"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
)

// TestController is a test. Yep.
func TestController(req *server.Request) {
	run := &server.Run{
		SSHConn: &server.SSHConnection{
			User: req.App.Config.MulchSuperUser,
			Host: "10.104.24.93",
			Port: 22,
			Auths: []ssh.AuthMethod{
				server.PublicKeyFile(req.App.Config.MulchSSHPrivateKey),
			},
			Log: req.Stream,
		},
		Tasks: []*server.RunTask{
			&server.RunTask{Script: "a1.sh"},
			&server.RunTask{Script: "a2.sh"},
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
func Test2Controller(req *server.Request) {
	req.Stream.Trace("test trace")
	req.Stream.Infof("your version: %s", req.HTTP.PostFormValue("version"))
	req.Stream.Info("test info")
	req.Stream.Warning("test warning")
	req.Stream.Error("test warning")

	req.Stream.Success("test success")
	req.Stream.Failure("test failure")
}
