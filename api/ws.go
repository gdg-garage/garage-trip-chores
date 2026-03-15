package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the dashboard
	},
}

type WsHub struct {
	logger     *slog.Logger
	clients    map[*websocket.Conn]bool
	broadcast  chan storage.Event
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.Mutex
}

func NewWsHub(logger *slog.Logger) *WsHub {
	return &WsHub{
		logger:     logger,
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan storage.Event),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *WsHub) Run() {
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
				client.Close()
			}
			h.mu.Unlock()
		case event := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				err := client.WriteJSON(event)
				if err != nil {
					h.logger.Error("websocket write error", "error", err)
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (api *Api) ServeWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		api.logger.Error("failed to upgrade websocket", "error", err)
		return
	}

	go func() {
		defer func() {
			api.hub.unregister <- conn
		}()

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		var authMsg struct {
			ApiKey string `json:"api_key"`
		}
		err = conn.ReadJSON(&authMsg)
		if err != nil || authMsg.ApiKey != api.conf.ApiKey {
			api.logger.Warn("websocket auth failed", "error", err)
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Unauthorized"))
			conn.Close()
			return
		}

		conn.SetReadDeadline(time.Time{})
		api.hub.register <- conn

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					api.logger.Error("websocket read error", "error", err)
				}
				break
			}
		}
	}()
}

func (h *WsHub) BroadcastEvent(event storage.Event) {
	h.broadcast <- event
}
