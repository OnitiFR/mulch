package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// LogController sends logs to client
func LogController(req *server.Request) {
	req.Stream.Infof("Hi! You are receiving live logs.")
	req.SetTarget(common.MessageAllTargets)
	// nothing to do, just wait forever…
	select {}
}

// GetLogHistoryController sends all "previous" log messages
func GetLogHistoryController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	// TODO: use req parameters for length & target
	messages := req.App.LogHistory.Search(50, common.MessageAllTargets)

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(messages)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
