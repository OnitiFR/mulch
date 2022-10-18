package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

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

	req.Stream.Successf("Secret '%s' defined", key)
}

// GetSecretController returns a secret
func GetSecretController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	key := req.SubPath

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

	var retData common.APISecretListEntries

	for _, name := range req.App.SecretsDB.GetKeys() {
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

	key := req.SubPath

	err := req.App.SecretsDB.Delete(key, req.APIKey.Comment)
	if err != nil {
		req.Stream.Failuref("Cannot delete secret: %s", err)
		return
	}

	req.Stream.Successf("Secret '%s' deleted", key)
}

// SyncSecretsController syncs secrets with another peer
func SyncSecretsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	jsonFile, _, err := req.HTTP.FormFile("db")
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	db := make(server.SecretDatabaseEntries)
	dec := json.NewDecoder(jsonFile)
	err = dec.Decode(&db)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	newer, err := req.App.SecretsDB.SyncWithDatabase(db)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(newer)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
