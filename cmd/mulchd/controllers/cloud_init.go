package controllers

import (
	"net/http"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// CloudInitController generates Cloud-Init meta-data and user-data
// request looks like: GET /cloud-init/<SecretUUID>/<revision>/<meta-data|user-data>
func CloudInitController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	parts := strings.Split(req.SubPath, "/")
	if len(parts) != 2 {
		errMsg := "request path is invalid"
		req.App.Log.Error(errMsg)
		http.Error(req.Response, errMsg, 400)
		return
	}

	uuid := parts[0]
	filename := parts[1]

	entry, err := req.App.VMDB.GetEntryBySecretUUID(uuid)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	log := server.NewLog(entry.VM.Config.Name, req.App.Hub, req.App.LogHistory)
	log.Infof("requesting cloud-init/%s", filename)

	metaData, userData, err := server.CloudInitDataGen(entry.VM, entry.Name, req.App)

	switch filename {
	case "meta-data":
		req.Println(metaData)
	case "user-data":
		req.Println(userData)
	default:
		errMsg := "invalid requested filename"
		log.Error(errMsg)
		http.Error(req.Response, errMsg, 400)
		return
	}
}
