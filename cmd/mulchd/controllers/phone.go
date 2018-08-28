package controllers

import (
	"net"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
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
