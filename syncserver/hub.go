package syncserver

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type realtimeMessage struct {
	Type       string `json:"type"`
	EntityType string `json:"entityType,omitempty"`
	EntityID   string `json:"entityId,omitempty"`
	NotebookID string `json:"notebookId,omitempty"`
	Sequence   int64  `json:"sequence,omitempty"`
}

type realtimeClient struct {
	userID string
	conn   *websocket.Conn
	send   chan []byte
}

type hub struct {
	mu      sync.RWMutex
	clients map[*realtimeClient]struct{}
}

func newHub() *hub { return &hub{clients: map[*realtimeClient]struct{}{}} }

func (h *hub) add(client *realtimeClient) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *hub) remove(client *realtimeClient) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
	_ = client.conn.Close()
}

func (h *hub) notifyUser(userID string, message realtimeMessage) {
	data, _ := json.Marshal(message)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.userID != userID {
			continue
		}
		select {
		case client.send <- data:
		default:
		}
	}
}

func (client *realtimeClient) writePump(done func()) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer done()
	for {
		select {
		case data, ok := <-client.send:
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
