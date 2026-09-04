package server

import (
	"os"
	"testing"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// TestMain shortens the hub send timeout once for the whole package,
// so tests exercising the "stalled client" paths stay fast. It's set
// here (and not per-test) because leaked hub.Run() goroutines would
// race with a per-test restore.
func TestMain(m *testing.M) {
	hubSendTimeout = 50 * time.Millisecond
	os.Exit(m.Run())
}

// startTestHub creates and runs a Hub. The Run() goroutine is leaked
// at test exit (Hub has no stop mechanism), it's harmless here.
func startTestHub(t *testing.T) *Hub {
	t.Helper()

	hub := NewHub(false)
	go hub.Run()
	return hub
}

// broadcastAsync returns a channel closed when Broadcast returned.
func broadcastAsync(hub *Hub, message *common.Message) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Broadcast(message)
	}()
	return done
}

// TestStalledClientDoesNotBlockBroadcast is the core regression test:
// a client that stops reading its Messages channel (ex: half-dead TCP
// peer behind the HTTP message stream) must not block the hub forever.
// Before the fix, Hub.Run() was stuck on an unbuffered send to the
// stalled client, freezing Broadcast — and every Log call of mulchd,
// including the SSH proxy accept loop.
func TestStalledClientDoesNotBlockBroadcast(t *testing.T) {
	hub := startTestHub(t)

	stalled := hub.Register("stalled", common.MessageNoTarget, false)
	_ = stalled // never reads its Messages channel
	reader := hub.Register("reader", common.MessageNoTarget, false)

	done := broadcastAsync(hub, common.NewMessage(common.MessageInfo, common.MessageNoTarget, "hello"))

	select {
	case msg := <-reader.Messages:
		if msg.Message != "hello" {
			t.Fatalf("reader got message '%s', expected 'hello'", msg.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("reader never received the message, hub is blocked by the stalled client")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Broadcast never returned, hub is blocked by the stalled client")
	}
}

// TestUnregisterDuringBlockedSendDoesNotDeadlock replays the other
// deadlock shape: a client stops reading and unregisters while
// Hub.Run() is (was) blocked sending to its Messages channel. Before
// the fix, Unregister was stuck sending on hub.unregister and Run()
// was stuck sending on client.Messages: mutual block, game over.
func TestUnregisterDuringBlockedSendDoesNotDeadlock(t *testing.T) {
	hub := startTestHub(t)

	client := hub.Register("quitter", common.MessageNoTarget, false)

	// make Run() enter the (timed-out) send to our client, then
	// unregister without ever reading Messages
	broadcastAsync(hub, common.NewMessage(common.MessageInfo, common.MessageNoTarget, "you won't read me"))

	unregistered := make(chan struct{})
	go func() {
		defer close(unregistered)
		client.Unregister()
	}()

	select {
	case <-unregistered:
	case <-time.After(3 * time.Second):
		t.Fatalf("Unregister never returned, hub is deadlocked")
	}

	// the hub must still be fully functional
	reader := hub.Register("reader", common.MessageNoTarget, false)
	broadcastAsync(hub, common.NewMessage(common.MessageInfo, common.MessageNoTarget, "still alive"))

	select {
	case msg := <-reader.Messages:
		if msg.Message != "still alive" {
			t.Fatalf("reader got message '%s', expected 'still alive'", msg.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("hub is not functional anymore after the unregister")
	}
}
