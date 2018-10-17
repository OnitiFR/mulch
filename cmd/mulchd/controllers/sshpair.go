package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
	"github.com/Xfennec/mulch/common"
)

// GetKeyPairController returns user key pair
func GetKeyPairController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	retData := &common.APISSHPair{
		Private: req.APIKey.SSHPrivate,
		Public:  req.APIKey.SSHPublic,
	}

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(retData)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}
