package controllers

import (
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// VersionController return versions
func VersionController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")
	req.Printf("server version: %s\n", server.Version)
	req.Printf("server protocol: %s\n", strconv.Itoa(server.ProtocolVersion))
}
