package controllers

import (
	"encoding/json"
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

// DeleteKeyController remove an API key from the DB
func DeleteKeyController(req *server.Request) {
	req.StartStream()
	keyComment := req.SubPath

	req.Stream.Info("deleting key")

	if req.APIKey.Comment == keyComment {
		req.Stream.Failure("Cannot delete your own key")
		return
	}

	err := req.App.APIKeysDB.Delete(keyComment)
	if err != nil {
		req.Stream.Failuref("Cannot delete Key: %s", err)
		return
	}

	req.Stream.Successf("Key '%s' deleted", keyComment)
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
	req.Response.Header().Set("Content-Type", "application/json")

	vmName := req.SubPath

	var retData common.APITrustListEntries

	for _, fp := range req.APIKey.SSHAllowedFingerprints {
		if vmName != "" && fp.VMName != vmName {
			continue
		}

		retData = append(retData, common.APITrustListEntry{
			VM:          fp.VMName,
			Fingerprint: fp.Fingerprint,
			AddedAt:     fp.AddedAt,
		})
	}

	// sort by VM name, then by date (newest first)
	sort.Slice(retData, func(i, j int) bool {
		if retData[i].VM == retData[j].VM {
			return retData[i].AddedAt.After(retData[j].AddedAt)
		}
		return retData[i].VM < retData[j].VM
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// AddKeyTrustedVMController add a VM to the trusted list of the current key
func AddKeyTrustedVMController(req *server.Request) {
	req.StartStream()

	vmName := req.SubPath
	fingerprint := req.HTTP.FormValue("fingerprint")

	_, err := req.App.VMDB.GetActiveByName(vmName)
	if err != nil {
		req.Stream.Failuref("cannot find VM: %s", err)
		return
	}

	fp := server.APISSHFingerprint{
		VMName:      vmName,
		Fingerprint: fingerprint,
	}
	err = req.App.APIKeysDB.AllowSSHFingerprint(req.APIKey, fp)
	if err != nil {
		req.Stream.Failuref("cannot forward key to VM: %s", err)
		return
	}

	req.Stream.Infof("key fingerprint: %s", fingerprint)
	req.Stream.Warning("reminder: this key can be used by other users on the VM when you are connected")
	req.Stream.Successf("key will now be forwared to VM '%s'", vmName)
}

// DeleteKeyTrustedVMController remove a VM from the trusted list of the current key
func DeleteKeyTrustedVMController(req *server.Request) {
	req.StartStream()

	vmName := req.SubPath
	fingerprint := req.HTTP.FormValue("fingerprint")

	if fingerprint == "" {
		req.App.APIKeysDB.RemoveAllSSHFingerprint(req.APIKey, vmName)
		req.Stream.Successf("all keys removed from VM '%s'", vmName)
		return
	}

	fp := server.APISSHFingerprint{
		VMName:      vmName,
		Fingerprint: fingerprint,
	}

	err := req.App.APIKeysDB.RemoveSSHFingerprint(req.APIKey, fp)
	if err != nil {
		req.Stream.Failuref("cannot remove key from VM: %s", err)
		return
	}

	req.Stream.Successf("key removed from VM '%s'", vmName)
}

// CleanKeyTrustedVMsController deleted all inexistant and inactive VMs from the trusted list of the current key
func CleanKeyTrustedVMsController(req *server.Request) {
	req.StartStream()

	sshAllowedFingerprints := make([]server.APISSHFingerprint, 0)

	for _, fp := range req.APIKey.SSHAllowedFingerprints {
		_, err := req.App.VMDB.GetActiveByName(fp.VMName)
		if err != nil {
			req.Stream.Infof("VM '%s' is inactive or deleted, removing key %s", fp.VMName, fp.Fingerprint)
		} else {
			sshAllowedFingerprints = append(sshAllowedFingerprints, fp)
		}
	}

	req.APIKey.SSHAllowedFingerprints = sshAllowedFingerprints
	err := req.App.APIKeysDB.Save()

	if err != nil {
		req.Stream.Failuref("cannot save: %s", err)
		return
	}

	req.Stream.Successf("forwarded keys cleaned")
}
