package controllers

import (
	"fmt"

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
	fmt.Println("hello from GetLogHistoryController")
	req.App.LogHistory.Dump()
}
