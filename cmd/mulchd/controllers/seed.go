package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
)

// ListSeedController lists seeds
func ListSeedController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	var retData common.APISeedListEntries

	for _, name := range req.App.Seeder.GetNames() {
		seed, err := req.App.Seeder.GetByName(name)
		if err != nil {
			msg := fmt.Sprintf("Seed '%s': %s", name, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		retData = append(retData, common.APISeedListEntry{
			Name:         name,
			Ready:        seed.Ready,
			LastModified: seed.LastModified,
		})
	}

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
