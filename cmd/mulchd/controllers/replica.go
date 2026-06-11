package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

	nameArg := req.SubPath
	if nameArg == "" {
		req.Stream.Failure("missing replica name")
		return
	}

	parsed, err := server.ParseVMName(nameArg)
	if err != nil {
		req.Stream.Failuref("invalid replica name '%s'", nameArg)
		return
	}

	revisionParam := req.HTTP.FormValue("revision")

	var vmID string
	switch {
	case revisionParam != "":
		// explicit --revision flag: the name must not also carry a revision
		if parsed.Revision != 0 {
			req.Stream.Failuref("revision given both in the name ('%s') and with --revision, use only one", nameArg)
			return
		}
		revision, err := strconv.Atoi(revisionParam)
		if err != nil {
			req.Stream.Failuref("invalid revision '%s'", revisionParam)
			return
		}
		vmID = server.NewVMName(parsed.Name, revision).ID()
	case parsed.Revision != 0:
		// revision embedded in the name (e.g. 'myvm-r2')
		vmID = nameArg
	default:
		// bare name: resolve among existing revisions, refuse if ambiguous
		matches := req.App.ReplicaDB.GetAllForName(parsed.Name)
		switch len(matches) {
		case 0:
			req.Stream.Failuref("no replica named '%s'", parsed.Name)
			return
		case 1:
			vmID = matches[0].ID()
		default:
			revs := make([]string, 0, len(matches))
			for _, m := range matches {
				revs = append(revs, strconv.Itoa(m.Revision))
			}
			sort.Strings(revs)
			req.Stream.Failuref("multiple revisions exist for '%s' (%s), select one with --revision", parsed.Name, strings.Join(revs, ", "))
			return
		}
	}

	if req.App.ReplicaDB.Get(vmID) == nil {
		req.Stream.Failuref("no replica named '%s'", vmID)
		return
	}

	if err := req.App.ReplicationReceiver.Delete(vmID); err != nil {
		req.Stream.Failuref("delete failed: %s", err)
		return
	}

	req.Stream.Successf("replica '%s' deleted", vmID)
}
