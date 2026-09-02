package server

import (
	"fmt"

	"github.com/OnitiFR/mulch/common"
)

// per-client message queue size. Must be large enough to absorb a
// burst from a verbose operation while the client is momentarily slow to read.
// A client that overflows its queue is considered gone, and dropped.
const hubClientQueueSize = 8192

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

				// non-blocking send: the hub must never wait for a client
				select {
				case client.Messages <- message:
				default:
					// removing the current key during a range is safe
					h.remove(client)
					// can't use Log here, it would broadcast… to us
					fmt.Printf("warning: hub: client '%s' stopped reading (%d messages queued), dropped\n",
						client.clientInfo, hubClientQueueSize)
				}
			}
		}
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
