package server

import (
	"bytes"
	"encoding/binary"
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

// buildMRPLStream serializes a valid MRPL stream (header + blocks + end sentinel).
func buildMRPLStream(diskSize uint64, blocks []mrplBlock) []byte {
	var b bytes.Buffer
	b.WriteString(ReplicationBlockMagic)
	binary.Write(&b, binary.BigEndian, ReplicationProtocolVersion)
	binary.Write(&b, binary.BigEndian, diskSize)
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
	stream := buildMRPLStream(size, []mrplBlock{
		{offset: 0, data: repeat('A', 512)},
		{offset: 1024, data: repeat('B', 256)},
	})

	got := make([]byte, size)
	total, err := readMRPLStream(bytes.NewReader(stream), size, func(off uint64, d []byte) error {
		copy(got[off:], d)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', 512)}})
	truncated := full[:len(full)-4] // drop part of the end sentinel

	_, err := readMRPLStream(bytes.NewReader(truncated), size, nil)
	if err == nil {
		t.Fatal("expected error on truncated stream, got nil")
	}
}

func TestReadMRPLStream_SizeMismatch(t *testing.T) {
	stream := buildMRPLStream(4096, nil)
	_, err := readMRPLStream(bytes.NewReader(stream), 8192, nil)
	if err == nil {
		t.Fatal("expected disk size mismatch error, got nil")
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

	stream := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(stream)); err != nil {
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

	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	truncated := full[:len(full)-4]
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(truncated)); err == nil {
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// incremental: overwrite the first 512 bytes with B
	inc := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", false, bytes.NewReader(inc)); err != nil {
		t.Fatalf("incremental: %s", err)
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// simulate a pre-flag persisted entry
	st := r.app.ReplicaDB.Get(vm)
	st.ConsistentSnapshot = false
	if err := r.app.ReplicaDB.Set(st); err != nil {
		t.Fatalf("Set: %s", err)
	}

	inc := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", false, bytes.NewReader(inc)); err != nil {
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	incFull := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('B', 512)}})
	torn := incFull[:len(incFull)-4]
	if err := r.ApplyBlocks(vm, "peerA", "config", false, bytes.NewReader(torn)); err == nil {
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(full)); err != nil {
		t.Fatalf("full copy: %s", err)
	}

	// simulate a crash mid-replay: a committed journal exists (B over first 512
	// bytes) and the .raw was only partially updated.
	journal := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('B', 512)}})
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
	full := buildMRPLStream(size, []mrplBlock{{offset: 0, data: repeat('A', size)}})
	if err := r.ApplyBlocks(vm, "peerA", "config", true, bytes.NewReader(full)); err != nil {
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
