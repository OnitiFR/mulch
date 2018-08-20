package main

// This Hub is derived from the Log hub (hub.go)

// PhoneCall describes a call from a VM
type PhoneCall struct {
	SecretUUID string
	RemoteIP   string
	CloutInit  bool
}

// PhoneHomeHubClient describes a client of an PhoneHomeHub
type PhoneHomeHubClient struct {
	PhoneCalls chan *PhoneCall
	Hub        *PhoneHomeHub
}

// PhoneHomeHub stores our internal channels and our client list
type PhoneHomeHub struct {
	clients    map[*PhoneHomeHubClient]bool
	broadcast  chan *PhoneCall
	register   chan *PhoneHomeHubClient
	unregister chan *PhoneHomeHubClient
}

// NewPhoneHomeHub creates a new PhoneHomeHub
func NewPhoneHomeHub() *PhoneHomeHub {
	h := &PhoneHomeHub{
		clients:    make(map[*PhoneHomeHubClient]bool),
		broadcast:  make(chan *PhoneCall),
		register:   make(chan *PhoneHomeHubClient),
		unregister: make(chan *PhoneHomeHubClient),
	}

	// start the Hub
	go func() {
		for {
			select {
			case client := <-h.register:
				h.clients[client] = true
				// fmt.Printf("new client: %s\n", client.clientInfo)
			case client := <-h.unregister:
				// fmt.Printf("del client: %s\n", client.clientInfo)
				if _, ok := h.clients[client]; ok {
					delete(h.clients, client)
					close(client.PhoneCalls)
				}
			case call := <-h.broadcast:
				// fmt.Printf("broadcasting\n")
				for client := range h.clients {
					client.PhoneCalls <- call
				}
			}

		}
	}()
	return h
}

// Broadcast a PhoneCall to all clients
func (h *PhoneHomeHub) Broadcast(call *PhoneCall) {
	h.broadcast <- call
}

// BroadcastPhoneCall broadcasts a PhoneCall using its details
func (h *PhoneHomeHub) BroadcastPhoneCall(secretUUID string, remoteIP string, cloudInit bool) {
	h.Broadcast(&PhoneCall{
		SecretUUID: secretUUID,
		RemoteIP:   remoteIP,
		CloutInit:  cloudInit,
	})
}

// Register will create PhoneHomeHubClient attached to the hub
func (h *PhoneHomeHub) Register() *PhoneHomeHubClient {
	client := &PhoneHomeHubClient{
		PhoneCalls: make(chan *PhoneCall),
		Hub:        h,
	}
	h.register <- client
	return client
}

// Unregister a client from the hub
func (hc *PhoneHomeHubClient) Unregister() {
	hc.Hub.unregister <- hc
}
