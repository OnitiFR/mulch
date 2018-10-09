package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
)

// ListKeysController list API keys
func ListKeysController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")
	keys := req.App.APIKeysDB.List()

	var retData common.APIKeyListEntries
	for _, key := range keys {

		retData = append(retData, common.APIKeyListEntry{
			Comment: key.Comment,
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
	keyComment := req.HTTP.FormValue("comment")
	newKey := req.App.APIKeysDB.GenKey()

	keyComment = strings.TrimSpace(keyComment)
	keys := req.App.APIKeysDB.List()
	for _, key := range keys {
		if key.Comment == keyComment {
			req.Stream.Failuref("Cannot create Key: duplicated comment '%s'", keyComment)
			return
		}
	}

	err := req.App.APIKeysDB.Add(keyComment, newKey)
	if err != nil {
		req.Stream.Failuref("Cannot create Key: %s", err)
		return
	}

	req.Stream.Infof("key = %s", newKey)
	req.Stream.Successf("Key '%s' created", keyComment)
}
