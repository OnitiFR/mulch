package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// ListGreenhouseVMsController list VMs in the greenhouse DB
func ListGreenhouseVMsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	vmNames := req.App.VMDB.GetGreenhouseNames()

	var retData common.APIVMListEntries
	for _, vmName := range vmNames {
		retData = append(retData, common.APIVMListEntry{
			Name:     vmName.Name,
			Revision: vmName.Revision,
			// Active:
			// LastIP:
			// State:
			// Locked:
			// WIP:
			// SuperUser:
			// AppUser:
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		if retData[i].Name == retData[j].Name {
			return retData[i].Revision < retData[j].Revision
		}
		return retData[i].Name < retData[j].Name
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}

}

// AbordGreenhouseVMController abort a VM creation
func AbordGreenhouseVMController(req *server.Request) {
	req.StartStream()

	vmName := req.SubPath
	revisionParams := req.HTTP.FormValue("revision")

	req.Stream.Infof("aborting %s", vmName)

	if vmName == "" {
		req.Stream.Failure("invalid VM name")
		return
	}

	var entry *server.VMDatabaseEntry

	// does a revision is specified?
	if revisionParams != "" {
		revision, err := strconv.Atoi(revisionParams)
		if err != nil {
			req.Stream.Failuref("invalid revision: %s", err.Error())
			return
		}

		name := server.NewVMName(vmName, revision)
		entry, err = req.App.VMDB.GetGreenhouseEntryByName(name)

		if err != nil {
			req.Stream.Failure(err.Error())
			return
		}
	} else {
		// no, find if there's only one VM with this name
		entries := req.App.VMDB.SearchGreenhouseEntries(vmName)

		if len(entries) == 0 {
			req.Stream.Failuref("no greenhouse VM with the name '%s'", vmName)
			return
		}

		if len(entries) > 1 {
			req.Stream.Failuref("several greenhouse VMs named '%s', please specify a revision (-r)", vmName)
			return
		}

		entry = entries[0]
	}

	/* Notes :
	- sometimes (depending on activity?) VMs does not respond to a graceful stop ("shutdown")
	- we've seen false failures with graceful stops (during a "sleep", for example, where VM goes away too fast and get-state fails)
	- … so we use a force stop
	*/

	err := server.VMStopByName(entry.Name, server.VMStopForce, 10*time.Second, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("unable to abort VM %s: %s", entry.Name, err)
		return
	}

	req.Stream.Successf("%s abort started (it can take up to a few minutes)", entry.Name)
}
