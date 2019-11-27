package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// CloudInitController generates Cloud-Init meta-data and user-data
// request looks like: GET /cloud-init/<SecretUUID>/<revision>/<meta-data|user-data>
func CloudInitController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	parts := strings.Split(req.SubPath, "/")
	if len(parts) != 3 {
		errMsg := "request path is invalid"
		req.App.Log.Error(errMsg)
		http.Error(req.Response, errMsg, 400)
		return
	}

	uuid := parts[0]
	revisionStr := parts[1]
	filename := parts[2]

	revision, _ := strconv.Atoi(revisionStr)

	// impossible, VM is not yet in DB :(
	vm, err := req.App.VMDB.GetBySecretUUID(uuid)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	metaData, userData, err := server.CloudInitDataGen(vm, revision, req.App)

	switch filename {
	case "meta-data":
		req.Println(metaData)
	case "user-data":
		req.Println(userData)
	default:
		errMsg := "invalid requested filename"
		req.App.Log.Error(errMsg)
		http.Error(req.Response, errMsg, 400)
		return
	}
}
