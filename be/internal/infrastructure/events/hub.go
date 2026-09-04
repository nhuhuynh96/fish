package events

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for IoT frontend
	},
}

type WSMessage struct {
	Type      string    `json:"type"` // "sensor_data", "sampling_event", "device_status"
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) Broadcast(msgType string, payload any) {
	msg := WSMessage{
		Type:      msgType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[Hub] json marshal error: %v", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Hub] write error, closing client: %v", err)
			client.Close()
			delete(h.clients, client)
		}
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Hub] websocket upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	log.Printf("[Hub] client connected (total: %d)", len(h.clients))

	// Reader loop to detect disconnect
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
			log.Printf("[Hub] client disconnected (total: %d)", len(h.clients))
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
