package web

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

type wsHub struct {
	clients    map[*wsClient]bool
	register   chan *wsClient
	unregister chan *wsClient
	broadcast  chan []byte
	mu         sync.Mutex
}

func newWsHub() *wsHub {
	return &wsHub{
		clients:    make(map[*wsClient]bool),
		register:   make(chan *wsClient),
		unregister: make(chan *wsClient),
		broadcast:  make(chan []byte, 64),
	}
}

func (h *wsHub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (w *Web) BroadcastWS(msg any) {
	bytes, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case w.wsHub.broadcast <- bytes:
	default:
	}
}

func (w *Web) handleWS(rw http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(rw, r, nil)
	if err != nil {
		w.logger.Error("failed to upgrade websocket", "error", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 64),
	}
	w.wsHub.register <- client

	// Send initial snapshot
	chores, _ := w.ListChoreViews(true)
	suggestionsMap := make(map[uint]SuggestionsResult)
	for _, c := range chores {
		sug := w.SuggestionsFor(storage.Chore{
			ID:                    c.ID,
			NecessaryCapabilities: stringsJoin(c.NecessaryCapabilities, ","),
		})
		suggestionsMap[c.ID] = sug
	}

	snapshot := map[string]any{
		"type":               "snapshot",
		"chores":             chores,
		"suggestions":        suggestionsMap,
		"upstream_connected": true,
	}
	if b, err := json.Marshal(snapshot); err == nil {
		client.send <- b
	}

	// Write pump
	go func() {
		defer func() {
			conn.Close()
		}()
		for message := range client.send {
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}()

	// Read pump
	go func() {
		defer func() {
			w.wsHub.unregister <- client
			conn.Close()
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

func stringsJoin(strs []string, sep string) string {
	res := ""
	for i, s := range strs {
		if i > 0 {
			res += sep
		}
		res += s
	}
	return res
}
