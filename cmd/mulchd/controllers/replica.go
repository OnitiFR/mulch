package controllers

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// replicaOrigin returns an identity string for the source of an incoming
// replication call: the comment of the authenticated API key.
func replicaOrigin(req *server.Request) string {
	if req.APIKey == nil {
		return ""
	}
	return req.APIKey.Comment
}

// ListReplicaController lists all replicas held by this peer (receiver side)
func ListReplicaController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	states := req.App.ReplicaDB.GetAll()

	entries := make(common.APIReplicaEntries, 0, len(states))
	for _, s := range states {
		entries = append(entries, common.APIReplicaEntry{
			Name:          s.Name,
			Revision:      s.Revision,
			Origin:        s.Origin,
			DiskSize:      s.DiskSize,
			LastUpdate:    s.LastUpdate,
			LastSyncBytes: s.LastSyncBytes,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Revision < entries[j].Revision
	})

	enc := json.NewEncoder(req.Response)
	err := enc.Encode(entries)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// DeleteReplicaController deletes a replica (database entry + raw file).
// The deletion is unconditional: if the source peer is still replicating this
// VM, the replica will reappear on the next prepare/sync.
func DeleteReplicaController(req *server.Request) {
	req.StartStream()

	vmID := req.SubPath
	if vmID == "" {
		req.Stream.Failure("missing replica name")
		return
	}

	if _, err := server.ParseVMName(vmID); err != nil {
		req.Stream.Failuref("invalid replica name '%s'", vmID)
		return
	}

	if req.App.ReplicaDB.Get(vmID) == nil {
		req.Stream.Failuref("no replica named '%s'", vmID)
		return
	}

	err := req.App.ReplicationReceiver.Delete(vmID)
	if err != nil {
		req.Stream.Failuref("delete failed: %s", err)
		return
	}

	req.Stream.Successf("replica '%s' deleted", vmID)
}
