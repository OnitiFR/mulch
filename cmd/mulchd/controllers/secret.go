package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// SecretController creates or updates a secret
func SetSecretController(req *server.Request) {
	req.StartStream()

	author := req.APIKey.Comment
	orgKey := req.SubPath
	value := req.HTTP.FormValue("value")

	key, err := req.App.SecretsDB.CleanKey(orgKey)
	if err != nil {
		req.Stream.Failuref("Invalid key: %s", err)
		return
	}

	err = req.App.SecretsDB.Set(key, value, author)
	if err != nil {
		req.Stream.Failuref("Cannot set secret: %s", err)
		return
	}

	vms, err := req.App.SecretsDB.GetAllVMsUsingSecret(key)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	if len(vms) > 0 {
		req.Stream.Warningf("the following VMs will need a restart (or rebuild): %s", strings.Join(vms, ", "))
	}

	req.Stream.Successf("Secret '%s' defined", key)
}

// GetSecretController returns a secret
func GetSecretController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	orgKey := req.SubPath
	key, err := req.App.SecretsDB.CleanKey(orgKey)
	if err != nil {
		errI := fmt.Errorf("invalid key: %s", err)
		req.App.Log.Error(errI.Error())
		http.Error(req.Response, errI.Error(), 400)
		return
	}

	secret, err := req.App.SecretsDB.Get(key)
	if err != nil {
		errS := fmt.Errorf("cannot get secret: %s", err)
		req.App.Log.Error(errS.Error())
		http.Error(req.Response, errS.Error(), 400)
		return
	}

	req.Println(secret.Value)
}

// ListSecretsController list all secrets
func ListSecretsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	path := req.HTTP.FormValue("path")

	var retData common.APISecretListEntries

	for _, name := range req.App.SecretsDB.GetKeys() {
		if path != "" && !strings.HasPrefix(name, path) {
			continue
		}

		secret, err := req.App.SecretsDB.Get(name)
		if err != nil {
			msg := fmt.Sprintf("Secret '%s': %s", name, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		retData = append(retData, common.APISecretListEntry{
			Key:       secret.Key,
			Modified:  secret.Modified,
			AuthorKey: secret.AuthorKey,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		return retData[i].Key < retData[j].Key
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// DeleteSecretController deletes a secret
func DeleteSecretController(req *server.Request) {
	req.StartStream()

	orgKey := req.SubPath
	key, err := req.App.SecretsDB.CleanKey(orgKey)
	if err != nil {
		req.Stream.Failuref("Invalid key: %s", err)
		return
	}

	vms, err := req.App.SecretsDB.GetAllVMsUsingSecret(key)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	if len(vms) > 0 {
		req.Stream.Failuref("Cannot delete secret, it's used by the following VMs: %s", strings.Join(vms, ", "))
		return
	}

	err = req.App.SecretsDB.Delete(key, req.APIKey.Comment)
	if err != nil {
		req.Stream.Failuref("Cannot delete secret: %s", err)
		return
	}

	req.Stream.Successf("Secret '%s' deleted", key)
}

// SyncSecretsController syncs secrets with another peer
func SyncSecretsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/octet-stream")

	// read content
	content, _, err := req.HTTP.FormFile("db")
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}
	defer content.Close()

	buff, err := io.ReadAll(content)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	// decrypt
	jsonFile, err := req.App.SecretsDB.Decrypt(buff)
	if err != nil {
		msg := fmt.Sprintf("Cannot decrypt secrets (%s), check that secret key matches both hosts", err)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 500)
		return
	}

	// unmarshal
	db := make(server.SecretDatabaseEntries)
	err = json.Unmarshal(jsonFile, &db)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	// sync
	newer, err := req.App.SecretsDB.SyncWithDatabase(db)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	// marshal response (to []byte)
	buff, err = json.Marshal(newer)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	// encrypt response
	encrypted, err := req.App.SecretsDB.Encrypt(buff)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	// write response
	_, err = req.Response.Write(encrypted)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// GetVMsUsingSecretsController returns a list of VMs using a secret
func GetVMsUsingSecretsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	orgKey := req.SubPath
	key, err := req.App.SecretsDB.CleanKey(orgKey)
	if err != nil {
		req.Stream.Failuref("Invalid key: %s", err)
		return
	}

	vms, err := req.App.SecretsDB.GetVMsUsingSecret(key)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(vms)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// ListSecretsUsageController returns a list of secrets with their usage count
func ListSecretsUsageController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	withPeersStr := req.HTTP.FormValue("with-peers")
	withPeers := false
	if withPeersStr == common.TrueStr {
		withPeers = true
	}

	retData, err := req.App.SecretsDB.GetSecretsUsage(withPeers)

	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
