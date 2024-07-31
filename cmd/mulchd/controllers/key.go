package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// ListKeysController list API keys
func ListKeysController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	comments := req.App.APIKeysDB.ListComments()

	var retData common.APIKeyListEntries
	for _, comment := range comments {

		retData = append(retData, common.APIKeyListEntry{
			Comment: comment,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		return retData[i].Comment < retData[j].Comment
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// NewKeyController creates and add a new API key to the DB
func NewKeyController(req *server.Request) {
	req.StartStream()
	keyComment := req.HTTP.FormValue("comment")
	keyComment = strings.TrimSpace(keyComment)

	req.Stream.Info("creating key")

	key, err := req.App.APIKeysDB.AddNew(keyComment)
	if err != nil {
		req.Stream.Failuref("Cannot create Key: %s", err)
		return
	}

	req.Stream.Infof("key = %s", key.Key)
	req.Stream.Successf("Key '%s' created", key.Comment)
}

// ListKeyRightsController list all rights of a specific key
func ListKeyRightsController(req *server.Request) {
	keyName := req.SubPath

	key := req.App.APIKeysDB.GetByComment(keyName)
	if key == nil {
		msg := "key not found"
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	req.Response.Header().Set("Content-Type", "application/json")

	var retData common.APIKeyRightEntries
	for _, right := range key.Rights {
		retData = append(retData, right.String())
	}

	// not sure about that, does it helps the user?
	sort.Slice(retData, func(i, j int) bool {
		return retData[i] < retData[j]
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}
}

// NewKeyRightController add a new right to the key
func NewKeyRightController(req *server.Request) {
	req.StartStream()

	keyName := req.SubPath

	key := req.App.APIKeysDB.GetByComment(keyName)
	if key == nil {
		req.Stream.Failuref("Cannot find key %s", keyName)
		return
	}

	rightStr := req.HTTP.FormValue("right")
	err := key.AddNewRight(rightStr)
	if err != nil {
		req.Stream.Failuref("Cannot add right: %s", err)
		return
	}

	err = req.App.APIKeysDB.Save()
	if err != nil {
		req.Stream.Failuref("Cannot save: %s", err)
		return
	}

	req.Stream.Successf("right added")
}

// DeleteKeyRightController remove a right from the key
func DeleteKeyRightController(req *server.Request) {
	req.StartStream()

	keyName := req.SubPath

	key := req.App.APIKeysDB.GetByComment(keyName)
	if key == nil {
		req.Stream.Failuref("Cannot find key %s", keyName)
		return
	}

	rightStr := req.HTTP.FormValue("right")
	err := key.RemoveRight(rightStr)
	if err != nil {
		req.Stream.Failuref("Cannot remove right: %s", err)
		return
	}

	err = req.App.APIKeysDB.Save()
	if err != nil {
		req.Stream.Failuref("Cannot save: %s", err)
		return
	}

	req.Stream.Successf("right removed")
}

// ListKeyTrustedVMsController list all trusted VMs for the current key
func ListKeyTrustedVMsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	if req.APIKey.TrustedVMs == nil || len(req.APIKey.TrustedVMs) == 0 {
		msg := fmt.Sprintf("no trusted VMs yet for key '%s'", req.APIKey.Comment)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
	}

	for vmName := range req.APIKey.TrustedVMs {
		req.Println(vmName)
	}
}

// AddKeyTrustedVMController add a VM to the trusted list of the current key
func AddKeyTrustedVMController(req *server.Request) {
	req.StartStream()

	vmName := req.SubPath

	_, err := req.App.VMDB.GetActiveByName(vmName)
	if err != nil {
		req.Stream.Failuref("Cannot trust VM: %s", err)
		return
	}

	err = req.App.APIKeysDB.AddTrustedVM(req.APIKey, vmName)
	if err != nil {
		req.Stream.Failuref("Cannot add VM to trusted list: %s", err)
		return
	}

	req.Stream.Warning("trusted VMs have access to your SSH agent, anyone with access to this VM will be able to use all your SSH keys while you are connected!")
	req.Stream.Successf("VM '%s' added to trusted list", vmName)
}

// DeleteKeyTrustedVMController remove a VM from the trusted list of the current key
func DeleteKeyTrustedVMController(req *server.Request) {
	req.StartStream()

	vmName := req.SubPath

	err := req.App.APIKeysDB.RemoveTrustedVM(req.APIKey, vmName)
	if err != nil {
		req.Stream.Failuref("Cannot remove VM from trusted list: %s", err)
		return
	}

	req.Stream.Successf("VM '%s' removed from trusted list", vmName)
}

// CleanKeyTrustedVMsController deleted all inexistant and inactive VMs from the trusted list of the current key
func CleanKeyTrustedVMsController(req *server.Request) {
	req.StartStream()

	if req.APIKey.TrustedVMs == nil || len(req.APIKey.TrustedVMs) == 0 {
		req.Stream.Successf("List is empty, nothing to clean")
		return
	}

	trustedVMs := make(map[string]bool)

	for vmName := range req.APIKey.TrustedVMs {
		_, err := req.App.VMDB.GetActiveByName(vmName)
		if err != nil {
			req.Stream.Infof("VM '%s' is inactive or deleted, removing from trusted list", vmName)
		} else {
			trustedVMs[vmName] = true
		}
	}

	req.APIKey.TrustedVMs = trustedVMs
	err := req.App.APIKeysDB.Save()

	if err != nil {
		req.Stream.Failuref("Cannot save: %s", err)
		return
	}

	req.Stream.Successf("Trusted VMs cleaned")
}
