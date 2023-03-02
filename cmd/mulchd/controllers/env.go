package controllers

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/alessio/shellescape"
)

// EnvController returns a text file with exported environment
func EnvController(req *server.Request) {
	instanceID := req.HTTP.FormValue("instance_id")

	if instanceID == "" {
		http.Error(req.Response, "missing instance_id", 400)
		return
	}

	entry, err := req.App.VMDB.GetEntryBySecretUUID(instanceID)

	if err != nil {
		http.Error(req.Response, "unknown instance_id", 404)
		return
	}

	env, err := entry.VM.GetEnvMap(entry.Name)

	if err != nil {
		req.App.Log.Errorf("error while getting env for VM %s: %s", entry.Name.Name, err)
	}

	if env == nil {
		http.Error(req.Response, "unable to get env map", 500)
		return
	}

	req.Response.Header().Set("Content-Type", "text/plain")
	req.Response.Write([]byte("# Created by Mulch, erased during boot (see TOML to add yours)\n\n"))

	if err != nil {
		req.Response.Write([]byte("# ERROR while getting env: " + err.Error() + "\n\n"))
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := env[key]
		str := fmt.Sprintf("export %s=%s\n", key, shellescape.Quote(val))
		req.Response.Write([]byte(str))
	}
}
