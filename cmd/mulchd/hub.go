package main

import "github.com/Xfennec/mulch"

type Hub struct {
	clients    map[*HubClient]bool
	broadcast  chan *mulch.Message
	register   chan *HubClient
	unregister chan *HubClient
}

type HubClient struct {
	Messages   chan *mulch.Message
	clientInfo string
	target     string
	hub        *Hub
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan *mulch.Message),
		register:   make(chan *HubClient),
		unregister: make(chan *HubClient),
		clients:    make(map[*HubClient]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			// fmt.Printf("new client: %s\n", client.ClientInfo)
		case client := <-h.unregister:
			// fmt.Printf("del client: %s\n", client.ClientInfo)
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Messages)
			}
		case message := <-h.broadcast:
			// fmt.Printf("broadcasting\n")
			for client := range h.clients {
				if client.target != message.Target && message.Target != mulch.MessageNoTarget {
					continue // not for this client
				}

				select {
				case client.Messages <- message:
				default:
					close(client.Messages)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) Broadcast(message *mulch.Message) {
	h.broadcast <- message
}

func (h *Hub) Register(info string, target string) *HubClient {
	client := &HubClient{
		Messages:   make(chan *mulch.Message),
		clientInfo: info,
		target:     target,
		hub:        h,
	}
	h.register <- client
	return client
}

func (hc *HubClient) Unregister() {
	hc.hub.unregister <- hc
}

func (hc *HubClient) SetTarget(target string) {
	hc.target = target
}
