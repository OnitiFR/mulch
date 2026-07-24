package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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
	syncing := req.App.ReplicationReceiver.SyncingIDs()

	entries := make(common.APIReplicaEntries, 0, len(states))
	seen := make(map[string]bool, len(states))
	for _, s := range states {
		id := s.ID()
		seen[id] = true

		status := "idle"
		switch {
		case s.Promoted:
			status = "promoted"
		case s.Promoting:
			status = "promoting"
		case syncing[id]:
			status = "syncing"
		}

		entries = append(entries, common.APIReplicaEntry{
			Name:               s.Name,
			Revision:           s.Revision,
			Origin:             s.Origin,
			Status:             status,
			DiskSize:           s.DiskSize,
			LastUpdate:         s.LastUpdate,
			LastSyncBytes:      s.LastSyncBytes,
			ConsistentSnapshot: s.ConsistentSnapshot,
		})
	}

	// inject syncing VMs not yet recorded in the database: this covers the
	// very first sync, where the replica is being received before any entry
	// exists (e.g. during prepare, or a sync against a not-yet-known VM).
	for id := range syncing {
		if seen[id] {
			continue
		}
		name, err := server.ParseVMName(id)
		if err != nil {
			continue
		}
		entries = append(entries, common.APIReplicaEntry{
			Name:     name.Name,
			Revision: name.Revision,
			Status:   "syncing",
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

// resolveReplicaID resolves a replica name with an optional revision (either
// embedded in the name, e.g. 'myvm-r2', or given through the "revision" form
// value) to an existing replica VM ID. A bare name is resolved among existing
// revisions and refused if ambiguous.
func resolveReplicaID(nameArg string, req *server.Request) (string, error) {
	parsed, err := server.ParseVMName(nameArg)
	if err != nil {
		return "", fmt.Errorf("invalid replica name '%s'", nameArg)
	}

	revisionParam := req.HTTP.FormValue("revision")

	var vmID string
	switch {
	case revisionParam != "":
		// explicit --revision flag: the name must not also carry a revision
		if parsed.Revision != 0 {
			return "", fmt.Errorf("revision given both in the name ('%s') and with --revision, use only one", nameArg)
		}
		revision, err := strconv.Atoi(revisionParam)
		if err != nil {
			return "", fmt.Errorf("invalid revision '%s'", revisionParam)
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
			return "", fmt.Errorf("no replica named '%s'", parsed.Name)
		case 1:
			vmID = matches[0].ID()
		default:
			revs := make([]string, 0, len(matches))
			for _, m := range matches {
				revs = append(revs, strconv.Itoa(m.Revision))
			}
			sort.Strings(revs)
			return "", fmt.Errorf("multiple revisions exist for '%s' (%s), select one with --revision", parsed.Name, strings.Join(revs, ", "))
		}
	}

	if req.App.ReplicaDB.Get(vmID) == nil {
		return "", fmt.Errorf("no replica named '%s'", vmID)
	}

	return vmID, nil
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

	vmID, err := resolveReplicaID(nameArg, req)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	if err := req.App.ReplicationReceiver.Delete(vmID); err != nil {
		req.Stream.Failuref("delete failed: %s", err)
		return
	}

	req.Stream.Successf("replica '%s' deleted", vmID)
}

// ActionReplicaController dispatches replica actions; "promote" is the only
// one for now.
func ActionReplicaController(req *server.Request) {
	req.StartStream()

	action := req.HTTP.FormValue("action")
	if action != "promote" {
		req.Stream.Failuref("missing or invalid action ('%s')", action)
		return
	}

	nameArg := req.SubPath
	if nameArg == "" {
		req.Stream.Failure("missing replica name")
		return
	}

	if req.HTTP.FormValue("force") != common.TrueStr {
		req.Stream.Failuref("promote is a dangerous failover action: see --help et re-run with --force to confirm")
		return
	}

	vmID, err := resolveReplicaID(nameArg, req)
	if err != nil {
		req.Stream.Failure(err.Error())
		return
	}

	req.SetTarget(vmID)

	operation := req.App.Operations.Add(&server.Operation{
		Origin:        req.APIKey.Comment,
		Action:        "promote",
		Ressource:     "replica",
		RessourceName: vmID,
	})
	defer req.App.Operations.Remove(operation)

	before := time.Now()
	vmName, err := server.PromoteReplica(vmID, req.APIKey.Comment, req.App, req.Stream)
	if err != nil {
		req.Stream.Failuref("promote failed: %s", err)
		return
	}
	after := time.Now()

	req.Stream.Successf("replica '%s' promoted as VM %s (%s)", vmID, vmName, after.Sub(before))
}
