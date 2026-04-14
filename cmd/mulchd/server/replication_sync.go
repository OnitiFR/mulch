package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// syncVM performs one sync cycle for a VM (full or incremental) using pull mode.
// QEMU exposes dirty blocks on a TCP NBD server (localhost), we read them and
// stream to the peer via HTTP.
func (rm *ReplicationManager) syncVM(vmName *VMName, vm *VM) {
	app := rm.app
	domainName := vmName.LibvirtDomainName(app)

	dom, err := app.Libvirt.GetDomainByName(domainName)
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("can't find domain: %s", err))
		return
	}
	if dom == nil {
		return
	}
	defer dom.Free()

	// check VM is running
	domState, _, err := dom.GetState()
	if err != nil || domState != libvirt.DOMAIN_RUNNING {
		return
	}

	// resolve actual disk device name from domain XML
	diskDev, err := getDiskTargetDev(dom)
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("can't resolve disk device: %s", err))
		return
	}
	app.Log.Tracef("replication %s: disk device is '%s'", vmName.ID(), diskDev)

	// determine sync mode
	state := app.ReplicationDB.Get(vmName.ID())
	needsFullCopy := rm.needsFullCopy(vmName, vm, dom, state)
	app.Log.Tracef("replication %s: needsFullCopy=%t", vmName.ID(), needsFullCopy)

	if needsFullCopy {
		// if the peer changed, clean up the old peer's replica first
		if state != nil && state.PeerName != "" && state.PeerName != vm.Config.ReplicationPeer {
			app.Log.Infof("replication %s: peer changed from '%s' to '%s', cleaning up old peer",
				vmName.ID(), state.PeerName, vm.Config.ReplicationPeer)
			rm.peerCleanup(state.PeerName, vmName)
		}

		// notify new peer to prepare (create raw file)
		err = rm.peerPrepare(vm, vmName)
		if err != nil {
			rm.recordError(vmName, fmt.Sprintf("peer prepare failed: %s", err))
			return
		}
	}

	// update status to syncing
	state = rm.ensureState(vmName, vm)
	state.Status = ReplicationSyncing
	app.ReplicationDB.Set(state)

	syncStart := time.Now()

	// FSFreeze with timeout
	app.Log.Tracef("replication %s: freezing guest filesystem", vmName.ID())
	frozen, err := rm.fsFreeze(dom, vmName)
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("FSFreeze failed: %s", err))
		return
	}
	app.Log.Tracef("replication %s: FSFreeze done (frozen=%t)", vmName.ID(), frozen)

	// create checkpoint (must always FSThaw even on error)
	newCpName := fmt.Sprintf("mulch-repl-%s-%d", vmName.ID(), time.Now().Unix())
	cpXML := rm.buildCheckpointXML(newCpName, diskDev)
	app.Log.Tracef("replication %s: creating checkpoint '%s'", vmName.ID(), newCpName)
	newCp, cpErr := dom.CreateCheckpointXML(cpXML, 0)

	// FSThaw immediately
	if frozen {
		thawErr := dom.FSThaw(nil, 0)
		if thawErr != nil {
			app.Log.Errorf("replication %s: FSThaw failed: %s", vmName.ID(), thawErr)
		} else {
			app.Log.Tracef("replication %s: FSThaw done", vmName.ID())
		}
	}

	if cpErr != nil {
		rm.recordError(vmName, fmt.Sprintf("checkpoint creation failed: %s", cpErr))
		return
	}
	defer newCp.Free()
	app.Log.Tracef("replication %s: checkpoint created", vmName.ID())

	// scratch file for pull-mode backup (libvirt creates it, handles DAC for QEMU)
	scratchPath := filepath.Join(app.Config.TempPath, "mulch-repl-scratch-"+vmName.ID()+".qcow2")
	defer os.Remove(scratchPath)

	// build pull-mode backup XML (TCP localhost, port auto-assigned by QEMU)
	previousCheckpoint := ""
	if !needsFullCopy && state.LastCheckpointName != "" {
		previousCheckpoint = state.LastCheckpointName
	}
	nbdPort, err := findFreePort()
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("can't find free TCP port: %s", err))
		return
	}
	backupXML := rm.buildPullBackupXML(diskDev, scratchPath, nbdPort, previousCheckpoint)

	if needsFullCopy {
		app.Log.Infof("replication %s: starting full copy to peer %s", vmName.ID(), vm.Config.ReplicationPeer)
	} else {
		app.Log.Tracef("replication %s: starting incremental sync to peer %s", vmName.ID(), vm.Config.ReplicationPeer)
	}

	// start pull-mode backup
	app.Log.Tracef("replication %s: starting BackupBegin (pull mode)", vmName.ID())
	err = dom.BackupBegin(backupXML, "", 0)
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("BackupBegin failed: %s", err))
		return
	}
	app.Log.Tracef("replication %s: BackupBegin succeeded", vmName.ID())

	// ensure we end the backup job even on error
	backupActive := true
	defer func() {
		if backupActive {
			dom.BlockJobAbort(diskDev, 0)
		}
	}()

	nbdAddress := fmt.Sprintf("localhost:%d", nbdPort)
	app.Log.Tracef("replication %s: NBD server at %s", vmName.ID(), nbdAddress)

	// pull dirty blocks from QEMU and stream them to the peer
	exportName := "mulch-repl"
	bitmapName := ""
	if !needsFullCopy {
		bitmapName = "mulch-repl-dirty"
	}

	syncBytes, err := rm.pullAndStreamBlocks(vm, vmName, nbdAddress, exportName, bitmapName, needsFullCopy)
	if err != nil {
		rm.recordError(vmName, fmt.Sprintf("pull and stream failed: %s", err))
		return
	}

	// end the backup job
	dom.BlockJobAbort(diskDev, 0)
	backupActive = false

	// delete old checkpoint (incremental mode only)
	if !needsFullCopy && state.LastCheckpointName != "" {
		rm.deleteCheckpoint(dom, state.LastCheckpointName, vmName)
	}

	// update state
	state.LastCheckpointName = newCpName
	state.FullCopyDone = true
	state.LastSyncTime = time.Now()
	state.LastSyncDuration = time.Since(syncStart)
	state.LastSyncBytes = syncBytes
	state.Status = ReplicationIdle
	state.LastError = ""
	state.ConsecutiveErrors = 0
	app.ReplicationDB.Set(state)

	if needsFullCopy {
		app.Log.Infof("replication %s: full copy completed (%s)", vmName.ID(), state.LastSyncDuration.Round(time.Second))
	} else {
		app.Log.Tracef("replication %s: incremental sync completed (%s)", vmName.ID(), state.LastSyncDuration.Round(time.Second))
	}
}

// needsFullCopy determines if a full copy is needed
func (rm *ReplicationManager) needsFullCopy(vmName *VMName, vm *VM, dom *libvirt.Domain, state *ReplicationState) bool {
	if state == nil || !state.FullCopyDone {
		return true
	}

	if state.LastCheckpointName == "" {
		return true
	}

	// peer changed?
	if state.PeerName != vm.Config.ReplicationPeer {
		return true
	}

	// checkpoint still exists on domain?
	cp, err := dom.CheckpointLookupByName(state.LastCheckpointName, 0)
	if err != nil {
		rm.app.Log.Warningf("replication %s: checkpoint '%s' not found on domain, full copy needed",
			vmName.ID(), state.LastCheckpointName)
		return true
	}
	cp.Free()

	return false
}

// fsFreeze freezes the guest filesystem with a timeout
func (rm *ReplicationManager) fsFreeze(dom *libvirt.Domain, vmName *VMName) (bool, error) {
	ch := make(chan error, 1)
	go func() {
		ch <- dom.FSFreeze(nil, 0)
	}()

	select {
	case err := <-ch:
		if err != nil {
			return false, err
		}
		return true, nil
	case <-time.After(ReplicationFSFreezeTimeout):
		return false, fmt.Errorf("FSFreeze timed out after %s", ReplicationFSFreezeTimeout)
	}
}

// buildCheckpointXML builds the XML for a new checkpoint
func (rm *ReplicationManager) buildCheckpointXML(name string, diskDev string) string {
	return fmt.Sprintf(`<domaincheckpoint>
  <name>%s</name>
  <disks>
    <disk name="%s" checkpoint="bitmap"/>
  </disks>
</domaincheckpoint>`, name, diskDev)
}

// buildPullBackupXML builds the XML for a pull-mode backup using libvirtxml structs.
// In pull mode, QEMU exposes the backup data on a TCP NBD server (localhost).
func (rm *ReplicationManager) buildPullBackupXML(diskDev string, scratchPath string, port uint, previousCheckpoint string) string {
	backup := &libvirtxml.DomainBackup{
		Incremental: previousCheckpoint,
		Pull: &libvirtxml.DomainBackupPull{
			Server: &libvirtxml.DomainBackupPullServer{
				TCP: &libvirtxml.DomainBackupPullServerTCP{
					Name: "localhost",
					Port: port,
				},
			},
			Disks: &libvirtxml.DomainBackupPullDisks{
				Disks: []libvirtxml.DomainBackupPullDisk{{
					Name:         diskDev,
					Backup:       "yes",
					ExportName:   "mulch-repl",
					ExportBitmap: "mulch-repl-dirty",
					Scratch: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: scratchPath,
						},
					},
				}},
			},
		},
	}

	xml, err := backup.Marshal()
	if err != nil {
		// fallback to raw XML if marshal fails (should not happen)
		return fmt.Sprintf(`<domainbackup mode="pull"><server name="localhost" port="%d"/><disks><disk name="%s" backup="yes" type="file" exportname="mulch-repl" exportbitmap="mulch-repl-dirty"><scratch file="%s"/></disk></disks></domainbackup>`, port, diskDev, scratchPath)
	}
	return xml
}

// findFreePort asks the kernel for an available TCP port on localhost.
func findFreePort() (uint, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return uint(port), nil
}

// pullAndStreamBlocks connects to QEMU's local NBD server, reads dirty blocks,
// and streams them to the peer via HTTP.
func (rm *ReplicationManager) pullAndStreamBlocks(vm *VM, vmName *VMName, nbdAddress string, exportName string, bitmapName string, fullCopy bool) (uint64, error) {
	peer, exists := rm.app.Config.Peers[vm.Config.ReplicationPeer]
	if !exists {
		return 0, fmt.Errorf("peer '%s' not found", vm.Config.ReplicationPeer)
	}

	// connect to QEMU's NBD server
	rm.app.Log.Tracef("replication %s: connecting NBD client to '%s' (export='%s', bitmap='%s')",
		vmName.ID(), nbdAddress, exportName, bitmapName)
	nbdClient, err := NewNBDClient(nbdAddress, exportName, bitmapName)
	if err != nil {
		return 0, fmt.Errorf("NBD client connect: %s", err)
	}
	defer nbdClient.Close()

	diskSize := nbdClient.ExportSize
	rm.app.Log.Tracef("replication %s: NBD connected, export size=%d bytes", vmName.ID(), diskSize)

	// set up a pipe: we write blocks into the pipe, PeerCall reads from it
	pipeReader, pipeWriter := io.Pipe()

	// stream result from goroutine
	type streamResult struct {
		bytes uint64
		err   error
	}
	streamCh := make(chan streamResult, 1)
	go func() {
		defer pipeWriter.Close()
		bytes, err := rm.writeBlockStream(pipeWriter, nbdClient, diskSize, fullCopy)
		streamCh <- streamResult{bytes, err}
	}()

	// send the stream to the peer
	call := &PeerCall{
		Peer:   peer,
		Method: "POST",
		Path:   "/replication/sync",
		Args: map[string]string{
			"vm_name": vmName.ID(),
		},
		UploadStream: &PeerCallStreamBody{
			ContentType: "application/octet-stream",
			Reader:      pipeReader,
		},
		TextCallback: func(body []byte) error {
			return nil // "OK" response
		},
		Log:     rm.app.Log,
		Libvirt: rm.app.Libvirt,
	}

	callErr := call.Do()

	// check streaming error
	result := <-streamCh
	if result.err != nil {
		return result.bytes, fmt.Errorf("writing block stream: %s", result.err)
	}
	if callErr != nil {
		return result.bytes, fmt.Errorf("peer sync call: %s", callErr)
	}

	return result.bytes, nil
}

// writeBlockStream writes the replication block protocol to w,
// reading blocks from the NBD client. Returns the number of data bytes streamed.
func (rm *ReplicationManager) writeBlockStream(w io.Writer, nbdClient *NBDClient, diskSize uint64, fullCopy bool) (uint64, error) {
	// write header
	if _, err := w.Write([]byte(ReplicationBlockMagic)); err != nil {
		return 0, err
	}
	if err := binary.Write(w, binary.BigEndian, ReplicationProtocolVersion); err != nil {
		return 0, err
	}
	if err := binary.Write(w, binary.BigEndian, diskSize); err != nil {
		return 0, err
	}

	var totalBytes uint64
	var blockCount uint64
	var zeroBlocks uint64
	buf := make([]byte, ReplicationMaxBlockSize)

	if fullCopy {
		rm.app.Log.Tracef("replication: writeBlockStream full copy, disk size=%d", diskSize)
		// read entire disk in chunks
		var offset uint64
		for offset < diskSize {
			length := uint32(ReplicationMaxBlockSize)
			if uint64(length) > diskSize-offset {
				length = uint32(diskSize - offset)
			}

			if err := nbdClient.Read(offset, length, buf[:length]); err != nil {
				return totalBytes, fmt.Errorf("NBD read at offset %d: %s", offset, err)
			}

			// skip all-zero blocks to avoid transferring unused disk space
			if !isZeroBlock(buf[:length]) {
				if err := writeBlock(w, offset, length, buf[:length]); err != nil {
					return totalBytes, err
				}
				totalBytes += uint64(length)
				blockCount++
			} else {
				zeroBlocks++
			}

			offset += uint64(length)
		}
	} else {
		rm.app.Log.Tracef("replication: writeBlockStream incremental, disk size=%d", diskSize)
		// incremental: use block status to find dirty extents
		var offset uint64
		var dirtyExtents uint64
		var cleanExtents uint64
		for offset < diskSize {
			queryLen := uint32(ReplicationMaxBlockSize)
			if uint64(queryLen) > diskSize-offset {
				queryLen = uint32(diskSize - offset)
			}

			extents, err := nbdClient.BlockStatus(offset, queryLen)
			if err != nil {
				return totalBytes, fmt.Errorf("NBD block status at offset %d: %s", offset, err)
			}

			for _, ext := range extents {
				if ext.Dirty {
					dirtyExtents++
					// dirty: read and send
					readLen := ext.Length
					readOff := offset

					// read in chunks if extent is larger than max block size
					for readLen > 0 {
						chunkLen := readLen
						if chunkLen > ReplicationMaxBlockSize {
							chunkLen = ReplicationMaxBlockSize
						}

						if err := nbdClient.Read(readOff, chunkLen, buf[:chunkLen]); err != nil {
							return totalBytes, fmt.Errorf("NBD read at offset %d: %s", readOff, err)
						}

						if err := writeBlock(w, readOff, chunkLen, buf[:chunkLen]); err != nil {
							return totalBytes, err
						}

						totalBytes += uint64(chunkLen)
						blockCount++
						readOff += uint64(chunkLen)
						readLen -= chunkLen
					}
				} else {
					cleanExtents++
				}
				offset += uint64(ext.Length)
			}

			if len(extents) == 0 {
				// no extents returned, advance by query length
				offset += uint64(queryLen)
			}
		}
		rm.app.Log.Tracef("replication: incremental extents: %d dirty, %d clean", dirtyExtents, cleanExtents)
	}

	// write end sentinel
	if err := binary.Write(w, binary.BigEndian, ReplicationEndSentinel); err != nil {
		return totalBytes, err
	}
	if err := binary.Write(w, binary.BigEndian, uint32(0)); err != nil {
		return totalBytes, err
	}

	if zeroBlocks > 0 {
		rm.app.Log.Tracef("replication: writeBlockStream done: %d blocks, %d bytes sent, %d zero blocks skipped", blockCount, totalBytes, zeroBlocks)
	} else {
		rm.app.Log.Tracef("replication: writeBlockStream done: %d blocks, %d bytes sent", blockCount, totalBytes)
	}

	return totalBytes, nil
}

// isZeroBlock returns true if all bytes in data are zero
func isZeroBlock(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

// writeBlock writes a single block entry to the replication stream
func writeBlock(w io.Writer, offset uint64, length uint32, data []byte) error {
	if err := binary.Write(w, binary.BigEndian, offset); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

// getDiskTargetDev returns the target device name (e.g. "vda") for the
// main mulch disk, by looking up the VMStorageAliasDisk alias in the domain XML.
func getDiskTargetDev(dom *libvirt.Domain) (string, error) {
	xmldoc, err := dom.GetXMLDesc(0)
	if err != nil {
		return "", fmt.Errorf("GetXMLDesc: %s", err)
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return "", fmt.Errorf("unmarshal domain XML: %s", err)
	}

	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			return disk.Target.Dev, nil
		}
	}

	return "", fmt.Errorf("disk with alias '%s' not found in domain XML", VMStorageAliasDisk)
}
