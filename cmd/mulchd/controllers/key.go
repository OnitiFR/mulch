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
