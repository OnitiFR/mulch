package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
)

// ListReplicationController lists all replication states
func ListReplicationController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	states := req.App.ReplicationDB.GetAll()

	entries := make(common.APIReplicationEntries, 0, len(states))
	for _, s := range states {
		vmName := server.NewVMName(s.Name, s.Revision)
		active, _ := req.App.VMDB.IsVMActive(vmName)

		entry := common.APIReplicationEntry{
			Name:              s.Name,
			Revision:          s.Revision,
			Active:            active,
			PeerName:          s.PeerName,
			Status:            string(s.Status),
			FullCopyDone:      s.FullCopyDone,
			LastSyncTime:      s.LastSyncTime,
			LastSyncDuration:  s.LastSyncDuration,
			LastSyncBytes:     s.LastSyncBytes,
			LastError:         s.LastError,
			LastErrorTime:     s.LastErrorTime,
			ConsecutiveErrors: s.ConsecutiveErrors,
			BackoffPaused:     s.ConsecutiveErrors >= server.ReplicationPauseErrors,
		}

		vm, err := req.App.VMDB.GetByName(vmName)
		if err == nil {
			entry.ConfiguredInterval = vm.Config.ReplicationInterval
			entry.BackoffInterval = req.App.ReplicationMgr.GetEffectiveInterval(vm, s)
		}

		entries = append(entries, entry)
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

// GetReplicationStatusController returns the replication state for a VM
func GetReplicationStatusController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/json")

	name := req.SubPath
	if name == "" {
		http.Error(req.Response, "missing VM name", 400)
		return
	}

	var state *server.ReplicationState
	revisionParam := req.HTTP.FormValue("revision")
	if revisionParam != "" {
		revision, err := strconv.Atoi(revisionParam)
		if err != nil {
			http.Error(req.Response, fmt.Sprintf("invalid revision '%s'", revisionParam), 400)
			return
		}
		vmName := server.NewVMName(name, revision)
		state = req.App.ReplicationDB.Get(vmName.ID())
	} else {
		activeEntry, err := req.App.VMDB.GetActiveEntryByName(name)
		if err == nil {
			state = req.App.ReplicationDB.Get(activeEntry.Name.ID())
		}
		if state == nil {
			state = req.App.ReplicationDB.GetByVMName(name)
		}
	}

	if state == nil {
		http.Error(req.Response, fmt.Sprintf("no replication state for VM '%s'", name), 404)
		return
	}

	vmName := server.NewVMName(state.Name, state.Revision)
	active, _ := req.App.VMDB.IsVMActive(vmName)

	entry := common.APIReplicationEntry{
		Name:              state.Name,
		Revision:          state.Revision,
		Active:            active,
		PeerName:          state.PeerName,
		Status:            string(state.Status),
		FullCopyDone:      state.FullCopyDone,
		LastSyncTime:      state.LastSyncTime,
		LastSyncDuration:  state.LastSyncDuration,
		LastSyncBytes:     state.LastSyncBytes,
		LastError:         state.LastError,
		LastErrorTime:     state.LastErrorTime,
		ConsecutiveErrors: state.ConsecutiveErrors,
		BackoffPaused:     state.ConsecutiveErrors >= server.ReplicationPauseErrors,
	}

	vm, err := req.App.VMDB.GetByName(vmName)
	if err == nil {
		entry.ConfiguredInterval = vm.Config.ReplicationInterval
		entry.BackoffInterval = req.App.ReplicationMgr.GetEffectiveInterval(vm, state)
	}

	enc := json.NewEncoder(req.Response)
	err = enc.Encode(entry)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// ActionReplicationController handles replication actions (sync, full-resync, disable, enable)
func ActionReplicationController(req *server.Request) {
	req.StartStream()
	vmNameStr := req.SubPath
	if vmNameStr == "" {
		req.Stream.Failure("missing VM name")
		return
	}

	action := req.HTTP.FormValue("action")
	if action == "" {
		req.Stream.Failure("missing 'action' parameter")
		return
	}

	vmName, err := server.ParseVMName(vmNameStr)
	if err != nil {
		req.Stream.Failuref("invalid VM name: %s", err)
		return
	}

	vm, err := req.App.VMDB.GetByName(vmName)
	if err != nil {
		req.Stream.Failuref("VM not found: %s", err)
		return
	}

	if vm.Config.ReplicationPeer == "" {
		req.Stream.Failure("replication is not configured for this VM")
		return
	}

	switch action {
	case "full-resync":
		req.App.ReplicationMgr.ResetFullCopy(vmName)
		req.Stream.Infof("replication for %s marked for full resync", vmName.ID())
	default:
		req.Stream.Failuref("unknown action '%s' (valid: full-resync)", action)
		return
	}

	req.Stream.Success("done")
}

// PrepareReplicationController creates or recreates a raw replica file (called by the source peer)
func PrepareReplicationController(req *server.Request) {
	req.StartStream()
	vmName := req.HTTP.FormValue("vm_name")
	if vmName == "" {
		req.Stream.Failure("missing 'vm_name' parameter")
		return
	}

	if _, err := server.ParseVMName(vmName); err != nil {
		req.Stream.Failuref("invalid VM name '%s'", vmName)
		return
	}

	diskSizeStr := req.HTTP.FormValue("disk_size")
	if diskSizeStr == "" {
		req.Stream.Failure("missing 'disk_size' parameter")
		return
	}

	diskSize, err := strconv.ParseUint(diskSizeStr, 10, 64)
	if err != nil {
		req.Stream.Failuref("invalid disk_size '%s': %s", diskSizeStr, err)
		return
	}

	err = req.App.ReplicationReceiver.Prepare(vmName, diskSize)
	if err != nil {
		req.Stream.Failuref("prepare failed: %s", err)
		return
	}

	req.Stream.Successf("replica prepared for '%s'", vmName)
}

// SyncReplicationController receives a binary block stream and applies it to the replica file
func SyncReplicationController(req *server.Request) {
	vmName := req.HTTP.FormValue("vm_name")
	if vmName == "" {
		http.Error(req.Response, "missing 'vm_name' parameter", 400)
		return
	}

	if _, err := server.ParseVMName(vmName); err != nil {
		http.Error(req.Response, fmt.Sprintf("invalid VM name '%s'", vmName), 400)
		return
	}

	err := req.App.ReplicationReceiver.ApplyBlocks(vmName, req.HTTP.Body)
	if err != nil {
		// drain remaining body so the HTTP 500 response reaches the source
		io.Copy(io.Discard, io.LimitReader(req.HTTP.Body, 1<<20))
		req.App.Log.Errorf("replication sync for '%s' failed: %s", vmName, err)
		http.Error(req.Response, err.Error(), 500)
		return
	}

	req.Response.Header().Set("Content-Type", "text/plain")
	req.Response.Write([]byte("OK"))
}

// CleanupReplicationController removes a replica file (called by the source peer on VM delete)
func CleanupReplicationController(req *server.Request) {
	req.StartStream()
	vmName := req.HTTP.FormValue("vm_name")
	if vmName == "" {
		req.Stream.Failure("missing 'vm_name' parameter")
		return
	}

	if _, err := server.ParseVMName(vmName); err != nil {
		req.Stream.Failuref("invalid VM name '%s'", vmName)
		return
	}

	err := req.App.ReplicationReceiver.Delete(vmName)
	if err != nil {
		req.Stream.Failuref("cleanup failed: %s", err)
		return
	}

	req.Stream.Successf("replica cleaned up for '%s'", vmName)
}
