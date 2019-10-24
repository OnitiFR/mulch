package controllers

import (
	"net"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// PhoneController receive "phone home" requests from instances
// (mulch client is not supposed to call this)
func PhoneController(req *server.Request) {
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
		req.Println("FAILED")
		return
	}

	instanceAnon := "?"
	if len(instanceID) > 4 {
		instanceAnon = instanceID[:4] + "…"
	}

	req.App.Log.Tracef("phoning: id=%s, ip=%s", instanceAnon, ip)
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
		entry, err := req.App.VMDB.GetEntryByVM(vm)
		if err != nil {
			req.App.Log.Errorf("unable to find VM: %s", err)
			return
		}
		log := server.NewLog(vm.Config.Name, req.App.Hub)
		log.Infof("phoning VM is %s - %s", entry.Name, ip)

		if vm.AssignedIPv4 != "" && vm.AssignedIPv4 != ip {
			log.Errorf("vm %s does not use it's assigned IP! (is '%s', should be '%s')", entry.Name, ip, vm.AssignedIPv4)
		}

		if vm.LastIP != ip {
			log.Warningf("vm IP changed since last call (from '%s' to '%s')", vm.LastIP, ip)

			vm.LastIP = ip
			err = req.App.VMDB.Update()
			if err != nil {
				log.Errorf("unable to update VM DB: %s", err)
			}
		}
		if req.HTTP.PostFormValue("dump_config") == common.TrueStr {
			req.Println(vm.Config.FileContent)
		}
	}

	req.App.PhoneHome.BroadcastPhoneCall(instanceID, ip, cloudInit)
}
