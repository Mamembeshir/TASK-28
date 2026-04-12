package sse

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type Event struct {
	Type string
	Data string
}

type Client struct {
	UserID uuid.UUID
	Events chan Event
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[uuid.UUID]map[*Client]struct{}),
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.UserID]; !ok {
		h.clients[client.UserID] = make(map[*Client]struct{})
	}
	h.clients[client.UserID][client] = struct{}{}
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.clients[client.UserID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.UserID)
		}
	}
	close(client.Events)
}

func (h *Hub) SendToUser(userID uuid.UUID, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.clients[userID]; ok {
		for client := range clients {
			select {
			case client.Events <- event:
			default:
				fmt.Printf("sse: dropping event for user %s (buffer full)\n", userID)
			}
		}
	}
}

func (h *Hub) Broadcast(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.clients {
		for client := range clients {
			select {
			case client.Events <- event:
			default:
			}
		}
	}
}
