package controllers

import (
	"github.com/OnitiFR/mulch/cmd/mulchd/server"
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
