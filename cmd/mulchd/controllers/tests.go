package controllers

import (
	"github.com/Xfennec/mulch/cmd/mulchd/server"
)

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

// Test3Controller is a device attachement test
func Test3Controller(req *server.Request) {
	vmName := req.SubPath
	err := server.VMAttachNewBackup(vmName, req.App, req.Stream)
	if err != nil {
		req.Stream.Error(err.Error())
		return
	}
	req.Stream.Success("OK")
}
