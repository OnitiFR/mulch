package controllers

import (
	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// ListPeersController list all configured peers
func ListPeersController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")
	for _, peer := range req.App.Config.Peers {
		req.Println(peer.Name)
	}
}
