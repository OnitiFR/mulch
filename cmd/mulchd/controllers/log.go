package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// LogController sends logs to client
func LogController(req *server.Request) {
	target := req.HTTP.FormValue("target")
	req.SetTarget(target)

	// nothing to do, just wait forever…
	select {}
}

// GetLogHistoryController sends all "previous" log messages
func GetLogHistoryController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	target := req.HTTP.FormValue("target")
	linesStr := req.HTTP.FormValue("lines")

	lines, err := strconv.Atoi(linesStr)
	if err != nil || lines < 1 || lines > 1000 {
		msg := fmt.Sprintf("invalid 'lines' value")
		req.App.Log.Error(msg)
		http.Error(req.Response, msg, 400)
		return
	}

	messages := req.App.LogHistory.Search(lines, target)

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(messages)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
