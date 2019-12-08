package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
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
			Size:         seed.Size,
			LastModified: seed.LastModified,
		})
	}

	sort.Slice(retData, func(i, j int) bool {
		return retData[i].Name < retData[j].Name
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(&retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// GetSeedStatusController is in charge of seed status command
func GetSeedStatusController(req *server.Request) {
	seedName := req.SubPath

	if seedName == "" {
		msg := fmt.Sprintf("no seed name given")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}

	seed, err := req.App.Seeder.GetByName(seedName)
	if err != nil {
		msg := fmt.Sprintf("seed '%s' not found", seedName)
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 404)
		return
	}

	data := &common.APISeedStatus{
		Name:       seedName,
		File:       seed.GetVolumeName(),
		Ready:      seed.Ready,
		URL:        seed.URL,
		Seeder:     seed.Seeder,
		Size:       seed.Size,
		Status:     seed.Status,
		StatusTime: seed.StatusTime,
	}

	req.Response.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(req.Response)
	err = enc.Encode(data)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// ActionSeedController redirect to the correct action for the seed
func ActionSeedController(req *server.Request) {
	req.StartStream()

	action := req.HTTP.FormValue("action")
	seedName := req.SubPath
	seed, err := req.App.Seeder.GetByName(seedName)

	if err != nil {
		req.Stream.Failuref("invalid seed '%s'", seedName)
		return
	}

	req.SetTarget(seed.Name)

	switch action {
	case "refresh":
		before := time.Now()
		err := seedRefresh(req, seed)
		after := time.Now()
		if err != nil {
			req.Stream.Failuref("refresh failed: %s", err)
		} else {
			req.Stream.Successf("refresh completed (%s)", after.Sub(before))
		}
	default:
		req.Stream.Failuref("missing or invalid action ('%s')", action)
		return
	}
}

// TODO: should check for conflitcs with any existing automatic seed operation
func seedRefresh(req *server.Request, seed *server.Seed) error {
	var err error
	if seed.URL != "" {
		err = req.App.Seeder.RefreshSeed(seed, server.SeedRefreshForce)
	}
	if seed.Seeder != "" {
		err = req.App.Seeder.RefreshSeeder(seed, server.SeedRefreshForce)
	}

	if err != nil {
		return err
	}
	return nil
}
