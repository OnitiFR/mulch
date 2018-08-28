package controllers

import (
	"fmt"
	"strconv"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
)

// VersionController return versions
func VersionController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")
	req.Response.Write([]byte(fmt.Sprintf("server version: %s\n", server.Version)))
	req.Response.Write([]byte(fmt.Sprintf("server protocol: %s\n", strconv.Itoa(server.ProtocolVersion))))
}
