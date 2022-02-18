package controllers

import (
	"net/http"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// LockedController will return "true" / "false" following vm.Locked value
func LockedController(req *server.Request) {
	instanceID := req.HTTP.FormValue("instance_id")

	if instanceID == "" {
		http.Error(req.Response, "missing instance_id", 400)
		return
	}

	entry, err := req.App.VMDB.GetEntryBySecretUUID(instanceID)

	if err != nil {
		http.Error(req.Response, "unknown instance_id", 404)
		return
	}

	vm := entry.VM
	req.Println(strconv.FormatBool(vm.Locked))
}
