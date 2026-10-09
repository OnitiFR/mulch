package server

import (
	"fmt"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// per-client message queue size. Must be large enough to absorb a
// burst from a verbose operation while the client is momentarily slow to read.
const hubClientQueueSize = 8192

// slots reserved, at the end of the queue, for messages we must never lose:
// SUCCESS/FAILURE carry the final status of an operation and decide the
// client exit code. They are only emitted by mulchd itself (never from a
// script output), so this reserve can't be flooded.
const hubClientReservedSlots = 256

// how long a client may stay in *continuous* overflow before we consider it
// really gone (and not just momentarily slow) and drop it.
const hubClientOverflowTimeout = 5 * time.Minute

// Hub structure allows multiple clients to receive messages
// from mulchd.
type Hub struct {
	clients    map[*HubClient]bool
	broadcast  chan *common.Message
	register   chan *HubClient
	unregister chan *HubClient
	trace      bool
}

// HubClient describes a client of a Hub
type HubClient struct {
	Messages   chan *common.Message
	clientInfo string
	target     string
	trace      bool
	hub        *Hub

	// overflow state, only ever touched by the Hub.Run() goroutine
	dropped       int
	overflowSince time.Time
}

// NewHub creates a new Hub
func NewHub(trace bool) *Hub {
	return &Hub{
		clients:    make(map[*HubClient]bool),
		broadcast:  make(chan *common.Message),
		register:   make(chan *HubClient),
		unregister: make(chan *HubClient),
		trace:      trace,
	}
}

// Run will start the Hub, allowing messages to be sent and received
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			// fmt.Printf("new client: %s\n", client.clientInfo)
		case client := <-h.unregister:
			// fmt.Printf("del client: %s\n", client.clientInfo)
			h.remove(client)
		case message := <-h.broadcast:
			// fmt.Printf("broadcasting\n")
			for client := range h.clients {
				if !message.MatchTarget(client.target, common.MessageMatchDefault) {
					continue // not for this client
				}
				if message.Type == common.MessageTrace && !client.trace {
					continue // this client don't want traces
				}

				h.send(client, message)
			}
		}
	}
}

// send a message to a client, never blocking.
//
// The hub must never wait for a client, whatever the reason it stopped
// reading (slow terminal, dead TCP connection, …). When a queue is full, we
// drop the *message* rather than the *client*: a burst of output is not a
// dead client. Losses are counted and reported to the client as soon as its
// queue drains. Only a client staying in overflow for
// hubClientOverflowTimeout is considered gone.
//
// Must only be called from the Run() goroutine.
func (h *Hub) send(client *HubClient, message *common.Message) {
	// SUCCESS/FAILURE end the operation client-side, they may use the
	// reserved slots at the end of the queue.
	critical := message.Type == common.MessageSuccess ||
		message.Type == common.MessageFailure

	if !critical && len(client.Messages) >= hubClientQueueSize-hubClientReservedSlots {
		h.dropMessage(client)
		return
	}

	h.reportLosses(client, message.Target)

	select {
	case client.Messages <- message:
	default:
		// only critical messages can reach this point (see above): the queue
		// is completely full, including the reserve.
		h.dropMessage(client)
	}
}

// dropMessage accounts for a dropped message, and drops the client itself if
// it has been unable to read anything for too long.
// Must only be called from the Run() goroutine.
func (h *Hub) dropMessage(client *HubClient) {
	if client.dropped == 0 {
		client.overflowSince = time.Now()
	}
	client.dropped++

	if time.Since(client.overflowSince) > hubClientOverflowTimeout {
		h.remove(client)
		// can't use Log here, it would broadcast… to us
		fmt.Printf("warning: hub: client '%s' stopped reading for %s (%d messages lost), dropped\n",
			client.clientInfo, hubClientOverflowTimeout, client.dropped)
	}
}

// reportLosses tells the client about previously lost messages, if we can do
// so without blocking. On failure, we'll simply try again on the next message.
// Must only be called from the Run() goroutine.
func (h *Hub) reportLosses(client *HubClient, target string) {
	if client.dropped == 0 {
		return
	}

	msg := common.NewMessage(common.MessageWarning, target, fmt.Sprintf(
		"… %d messages skipped (output too fast for this client) …", client.dropped))

	select {
	case client.Messages <- msg:
		client.dropped = 0
		client.overflowSince = time.Time{}
	default:
	}
}

// remove a client from the hub and close its queue. The closed
// channel is how the client learns it's gone.
// Must only be called from the Run() goroutine.
func (h *Hub) remove(client *HubClient) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.Messages)
	}
}

// Broadcast send a message to all clients of the Hub
// (if the target matches)
func (h *Hub) Broadcast(message *common.Message) {
	h.broadcast <- message
}

// Register a new client of the Hub
// clientInfo is not currently used but is supposed to differentiate
// the client. Target may be common.MessageNoTarget.
func (h *Hub) Register(info string, target string, trace bool) *HubClient {
	client := &HubClient{
		Messages:   make(chan *common.Message, hubClientQueueSize),
		clientInfo: info,
		target:     target,
		trace:      trace,
		hub:        h,
	}
	h.register <- client
	return client
}

// Unregister the client from the Hub
func (hc *HubClient) Unregister() {
	hc.hub.unregister <- hc
}

// SetTarget allows the client to change (receiving) target
func (hc *HubClient) SetTarget(target string) {
	hc.target = target
}
