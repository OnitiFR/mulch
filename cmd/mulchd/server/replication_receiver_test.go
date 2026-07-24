package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// --- test helpers ---

type mrplBlock struct {
	offset uint64
	data   []byte
}

// buildMRPLStream serializes a valid MRPL stream (header + config + blocks +
// end sentinel).
func buildMRPLStream(diskSize uint64, config string, blocks []mrplBlock) []byte {
	var b bytes.Buffer
	b.WriteString(ReplicationBlockMagic)
	binary.Write(&b, binary.BigEndian, ReplicationProtocolVersion)
	binary.Write(&b, binary.BigEndian, diskSize)
	binary.Write(&b, binary.BigEndian, uint32(len(config)))
	b.WriteString(config)
	for _, bl := range blocks {
		binary.Write(&b, binary.BigEndian, bl.offset)
		binary.Write(&b, binary.BigEndian, uint32(len(bl.data)))
		b.Write(bl.data)
	}
	binary.Write(&b, binary.BigEndian, ReplicationEndSentinel)
	binary.Write(&b, binary.BigEndian, uint32(0))
	return b.Bytes()
}

func repeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func newTestReceiver(t *testing.T) *ReplicationReceiver {
	t.Helper()

	hub := NewHub(false)
	go hub.Run()

	app := &App{
		Log:       NewLog("", hub, NewLogHistory(100)),
		ReplicaDB: nil,
	}
	db, err := NewReplicaDatabase(filepath.Join(t.TempDir(), "replica.db"))
	if err != nil {
		t.Fatalf("NewReplicaDatabase: %s", err)
	}
	app.ReplicaDB = db

	return &ReplicationReceiver{
		app:         app,
		replicaPath: t.TempDir(),
		fileLocks:   make(map[string]*sync.Mutex),
		syncing:     make(map[string]bool),
	}
}

func readRaw(t *testing.T, r *ReplicationReceiver, vmName string) []byte {
	t.Helper()
	data, err := os.ReadFile(r.filePath(vmName))
	if err != nil {
		t.Fatalf("reading raw: %s", err)
	}
	return data
}

// --- readMRPLStream ---

func TestReadMRPLStream_Complete(t *testing.T) {
	const size = 4096
	stream := buildMRPLStream(size, "config", []mrplBlock{
		{offset: 0, data: repeat('A', 512)},
		{offset: 1024, data: repeat('B', 256)},
	})

	got := make([]byte, size)
	config, total, err := readMRPLStream(bytes.NewReader(stream), size, func(off uint64, d []byte) error {
		copy(got[off:], d)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if config != "config" {
		t.Fatalf("config = %q, want %q", config, "config")
	}
	if total != 512+256 {
		t.Fatalf("total = %d, want %d", total, 512+256)
	}
	if !bytes.Equal(got[0:512], repeat('A', 512)) || !bytes.Equal(got[1024:1280], repeat('B', 256)) {
		t.Fatal("applied bytes mismatch")
	}
}

// A premature EOF (no end sentinel) must be reported as an error: this is what
// distinguishes a torn stream (source crashed mid-copy) from a finished one.
func TestReadMRPLStream_TruncatedIsError(t *testing.T) {
	const size = 4096
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', 512)}})
	truncated := full[:len(full)-4] // drop part of the end sentinel

	_, _, err := readMRPLStream(bytes.NewReader(truncated), size, nil)
	if err == nil {
		t.Fatal("expected error on truncated stream, got nil")
	}
}

func TestReadMRPLStream_SizeMismatch(t *testing.T) {
	stream := buildMRPLStream(4096, "config", nil)
	_, _, err := readMRPLStream(bytes.NewReader(stream), 8192, nil)
	if err == nil {
		t.Fatal("expected disk size mismatch error, got nil")
	}
}

// A corrupt stream announcing a huge config must be rejected before any
// allocation is attempted.
func TestReadMRPLStream_ConfigTooLarge(t *testing.T) {
	const size = 4096
	var b bytes.Buffer
	b.WriteString(ReplicationBlockMagic)
	binary.Write(&b, binary.BigEndian, ReplicationProtocolVersion)
	binary.Write(&b, binary.BigEndian, uint64(size))
	binary.Write(&b, binary.BigEndian, uint32(ReplicationMaxConfigSize+1))

	_, _, err := readMRPLStream(bytes.NewReader(b.Bytes()), size, nil)
	if err == nil {
		t.Fatal("expected config too large error, got nil")
	}
}

// --- full copy ---

func TestApplyFullCopy_SetsConsistentSnapshot(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	if st := r.app.ReplicaDB.Get(vm); st == nil || st.ConsistentSnapshot {
		t.Fatal("ConsistentSnapshot should be false right after Prepare")
	}

	stream := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(stream)); err != nil {
		t.Fatalf("ApplyBlocks (full): %s", err)
	}

	if st := r.app.ReplicaDB.Get(vm); st == nil || !st.ConsistentSnapshot {
		t.Fatal("ConsistentSnapshot should be true after a completed full copy")
	}
	if !bytes.Equal(readRaw(t, r, vm), repeat('A', size)) {
		t.Fatal("raw content mismatch after full copy")
	}
}

// An interrupted full copy must leave ConsistentSnapshot false (the .raw is partial).
func TestApplyFullCopy_TruncatedKeepsInconsistent(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}

	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	truncated := full[:len(full)-4]
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(truncated)); err == nil {
		t.Fatal("expected error on truncated full copy")
	}

	if st := r.app.ReplicaDB.Get(vm); st == nil || st.ConsistentSnapshot {
		t.Fatal("ConsistentSnapshot must stay false after an interrupted full copy")
	}
}

// --- incremental: staging + atomic commit ---

func TestApplyIncremental_AppliesDelta(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// incremental: overwrite the first 512 bytes with B (with an updated VM
	// config, to check the stream header refreshes the database entry)
	inc := buildMRPLStream(size, "config-v2", []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	if err := r.ApplyBlocks(vm, "peerA", false, bytes.NewReader(inc)); err != nil {
		t.Fatalf("incremental: %s", err)
	}

	if st := r.app.ReplicaDB.Get(vm); st == nil || st.Config != "config-v2" {
		t.Fatal("replica Config should have been refreshed from the stream header")
	}

	raw := readRaw(t, r, vm)
	if !bytes.Equal(raw[0:512], repeat('B', 512)) || !bytes.Equal(raw[512:], repeat('A', size-512)) {
		t.Fatal("raw content mismatch after incremental")
	}
	// no journal artefacts must remain, and Applying must be cleared
	assertNoJournalArtefacts(t, r, vm)
	if st := r.app.ReplicaDB.Get(vm); st == nil || st.Applying {
		t.Fatal("Applying should be false after a completed incremental")
	}
	if st := r.app.ReplicaDB.Get(vm); st == nil || !st.ConsistentSnapshot {
		t.Fatal("ConsistentSnapshot should be true after a completed incremental")
	}
}

// A replica entry persisted before the flag existed loads with
// ConsistentSnapshot=false; the next successful incremental must re-arm it
// (the image is at a complete FSFreeze point).
func TestApplyIncremental_ReArmsConsistentSnapshot(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// simulate a pre-flag persisted entry
	st := r.app.ReplicaDB.Get(vm)
	st.ConsistentSnapshot = false
	if err := r.app.ReplicaDB.Set(st); err != nil {
		t.Fatalf("Set: %s", err)
	}

	inc := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	if err := r.ApplyBlocks(vm, "peerA", false, bytes.NewReader(inc)); err != nil {
		t.Fatalf("incremental: %s", err)
	}

	if st := r.app.ReplicaDB.Get(vm); st == nil || !st.ConsistentSnapshot {
		t.Fatal("ConsistentSnapshot should have been re-armed by the incremental")
	}
}

// A torn incremental (source crashes mid-stream) must NOT corrupt the .raw: it
// stays at the previous consistent point and no journal is left behind.
func TestApplyIncremental_TornDoesNotCorruptRaw(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	incFull := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	torn := incFull[:len(incFull)-4]
	if err := r.ApplyBlocks(vm, "peerA", false, bytes.NewReader(torn)); err == nil {
		t.Fatal("expected error on torn incremental")
	}

	if !bytes.Equal(readRaw(t, r, vm), repeat('A', size)) {
		t.Fatal("raw must stay at the previous consistent point after a torn incremental")
	}
	assertNoJournalArtefacts(t, r, vm)
}

// --- recovery at boot ---

// A committed-but-unapplied journal must be replayed at boot, idempotently
// (here the .raw was only partially updated when the crash happened).
func TestRecoverJournals_ReplaysCommittedJournal(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// simulate a crash mid-replay: a committed journal exists (B over first 512
	// bytes) and the .raw was only partially updated.
	journal := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	if err := os.WriteFile(r.journalPath(vm), journal, 0600); err != nil {
		t.Fatalf("writing journal: %s", err)
	}
	rawFile, _ := os.OpenFile(r.filePath(vm), os.O_RDWR, 0600)
	rawFile.WriteAt(repeat('B', 200), 0) // partial apply
	rawFile.Close()
	r.setApplying(vm, true)

	if err := r.recoverJournals(); err != nil {
		t.Fatalf("recoverJournals: %s", err)
	}

	raw := readRaw(t, r, vm)
	if !bytes.Equal(raw[0:512], repeat('B', 512)) || !bytes.Equal(raw[512:], repeat('A', size-512)) {
		t.Fatal("journal replay did not produce the expected consistent state")
	}
	assertNoJournalArtefacts(t, r, vm)
	if st := r.app.ReplicaDB.Get(vm); st == nil || st.Applying {
		t.Fatal("Applying should be cleared after replay")
	}
}

// An incomplete .part (spool never committed) must be discarded, leaving the
// .raw untouched.
func TestRecoverJournals_DiscardsPart(t *testing.T) {
	r := newTestReceiver(t)
	const vm = "testvm"
	const size = 4096

	if err := r.Prepare(vm, size, "peerA", "config"); err != nil {
		t.Fatalf("Prepare: %s", err)
	}
	full := buildMRPLStream(size, "config", []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	if err := os.WriteFile(r.partPath(vm), []byte("garbage spool"), 0600); err != nil {
		t.Fatalf("writing part: %s", err)
	}

	if err := r.recoverJournals(); err != nil {
		t.Fatalf("recoverJournals: %s", err)
	}

	if _, err := os.Stat(r.partPath(vm)); !os.IsNotExist(err) {
		t.Fatal("stale .part should have been discarded")
	}
	if !bytes.Equal(readRaw(t, r, vm), repeat('A', size)) {
		t.Fatal("raw must be untouched after discarding a .part")
	}
}

func assertNoJournalArtefacts(t *testing.T, r *ReplicationReceiver, vm string) {
	t.Helper()
	if _, err := os.Stat(r.partPath(vm)); !os.IsNotExist(err) {
		t.Fatalf("unexpected .part file for '%s'", vm)
	}
	if _, err := os.Stat(r.journalPath(vm)); !os.IsNotExist(err) {
		t.Fatalf("unexpected .journal file for '%s'", vm)
	}
}

// --- checkPromoted ---

// newTestVMDB returns a bare in-memory VMDatabase (no file persistence),
// enough for name-existence checks.
func newTestVMDB() *VMDatabase {
	return &VMDatabase{
		db:           make(map[string]*VMDatabaseEntry),
		greenhouseDB: make(map[string]*VMDatabaseEntry),
	}
}

// A promoted tombstone stands while the promoted VM exists (whatever the
// revision), and is dropped once the VM is gone (failback: the source must be
// able to resume replication without being stood down by a stale tombstone).
func TestCheckPromoted_StaleTombstoneDropped(t *testing.T) {
	r := newTestReceiver(t)
	r.app.VMDB = newTestVMDB()

	tomb := &ReplicaState{Name: "vm1", Revision: 1, Promoted: true}
	if err := r.app.ReplicaDB.Set(tomb); err != nil {
		t.Fatalf("Set: %s", err)
	}

	vmName := NewVMName("vm1", 2)
	r.app.VMDB.db[vmName.ID()] = &VMDatabaseEntry{Name: vmName, Active: true}

	if err := r.checkPromoted("vm1-r3"); !errors.Is(err, ErrReplicaPromoted) {
		t.Fatalf("expected ErrReplicaPromoted while the VM exists, got: %v", err)
	}
	if r.app.ReplicaDB.Get(tomb.ID()) == nil {
		t.Fatal("tombstone must be kept while the promoted VM exists")
	}

	delete(r.app.VMDB.db, vmName.ID())

	if err := r.checkPromoted("vm1-r3"); err != nil {
		t.Fatalf("expected replication to be accepted again, got: %v", err)
	}
	if r.app.ReplicaDB.Get(tomb.ID()) != nil {
		t.Fatal("stale tombstone should have been dropped")
	}
}

// A "promoting" entry is never dropped: the VM does not exist yet in the VMDB
// during the promote.
func TestCheckPromoted_PromotingIsNeverDropped(t *testing.T) {
	r := newTestReceiver(t)
	r.app.VMDB = newTestVMDB()

	entry := &ReplicaState{Name: "vm1", Revision: 1, Promoting: true}
	if err := r.app.ReplicaDB.Set(entry); err != nil {
		t.Fatalf("Set: %s", err)
	}

	if err := r.checkPromoted("vm1-r1"); !errors.Is(err, ErrReplicaPromoted) {
		t.Fatalf("expected ErrReplicaPromoted during a promote, got: %v", err)
	}
	if r.app.ReplicaDB.Get(entry.ID()) == nil {
		t.Fatal("promoting entry must never be dropped")
	}
}
