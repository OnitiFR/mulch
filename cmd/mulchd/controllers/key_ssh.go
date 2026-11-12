package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/ssh"
)

// ListKeySSHController lists personal SSH keys of an API key
func ListKeySSHController(req *server.Request) {
	key := req.App.APIKeysDB.GetByComment(req.SubPath)
	if key == nil {
		msg := "key not found"
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	retData := common.APIKeySSHEntries{}
	for _, authKey := range key.SSHAuthorizedKeys {
		pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(authKey.PublicKey))
		if err != nil {
			req.App.Log.Errorf("key '%s': invalid SSH key: %s", key.Comment, err)
			continue
		}
		retData = append(retData, common.APIKeySSHEntry{
			Type:        pub.Type(),
			Fingerprint: ssh.FingerprintSHA256(pub),
			Comment:     comment,
			AddedAt:     authKey.AddedAt,
		})
	}

	req.Response.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// AddKeySSHController adds a personal SSH key to an API key
func AddKeySSHController(req *server.Request) {
	req.StartStream()

	key := req.App.APIKeysDB.GetByComment(req.SubPath)
	if key == nil {
		req.Stream.Failuref("Cannot find key %s", req.SubPath)
		return
	}

	pub, err := req.App.APIKeysDB.AddSSHAuthorizedKey(key, req.HTTP.FormValue("pubkey"))
	if err != nil {
		req.Stream.Failuref("Cannot add SSH key: %s", err)
		return
	}

	req.App.AlertSender.Security("SSH key added", fmt.Sprintf("SSH key %s added to '%s' by %s",
		ssh.FingerprintSHA256(pub), key.Comment, req.Author()))

	req.Stream.Infof("fingerprint: %s", ssh.FingerprintSHA256(pub))
	req.Stream.Successf("SSH key added to '%s'", key.Comment)
}

// DeleteKeySSHController removes a personal SSH key from an API key
func DeleteKeySSHController(req *server.Request) {
	req.StartStream()

	key := req.App.APIKeysDB.GetByComment(req.SubPath)
	if key == nil {
		req.Stream.Failuref("Cannot find key %s", req.SubPath)
		return
	}

	fingerprint := req.HTTP.FormValue("fingerprint")
	err := req.App.APIKeysDB.RemoveSSHAuthorizedKey(key, fingerprint)
	if err != nil {
		req.Stream.Failuref("Cannot remove SSH key: %s", err)
		return
	}

	req.Stream.Info("existing sessions using this SSH key are revoked")
	req.Stream.Successf("SSH key removed from '%s'", key.Comment)
}

// SetKeySSHOnlyController enables or disables SSH-only mode of an API key
func SetKeySSHOnlyController(req *server.Request) {
	req.StartStream()

	key := req.App.APIKeysDB.GetByComment(req.SubPath)
	if key == nil {
		req.Stream.Failuref("Cannot find key %s", req.SubPath)
		return
	}

	var enable bool
	switch req.HTTP.FormValue("enable") {
	case "true":
		enable = true
	case "false":
		enable = false
	default:
		req.Stream.Failure("'enable' must be 'true' or 'false'")
		return
	}

	wasEnabled := key.SSHAuthOnly
	err := req.App.APIKeysDB.SetSSHAuthOnly(key, enable)
	if err != nil {
		req.Stream.Failuref("Cannot change SSH-only mode: %s", err)
		return
	}

	if wasEnabled && !enable {
		req.App.AlertSender.Security("SSH-only mode disabled", fmt.Sprintf("SSH-only mode disabled for '%s' by %s (static secrets are valid again)",
			key.Comment, req.Author()))
	}

	if enable {
		req.Stream.Info("static API key and generated SSH key pair (sshpair) are now disabled")
		if key == req.APIKey {
			req.Stream.Warning("this is your own key: use 'ssh_pub_key' in your client configuration from now on")
		}
		req.Stream.Successf("SSH-only mode enabled for '%s'", key.Comment)
	} else {
		req.Stream.Warning("static API key and generated SSH key pair (sshpair) are enabled again")
		req.Stream.Successf("SSH-only mode disabled for '%s'", key.Comment)
	}
}
