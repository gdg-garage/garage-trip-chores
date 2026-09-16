package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

const (
	writeWait      = 10 * time.Second
	sendBufferSize = 64
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the dashboard
	},
}

type WsClient struct {
	hub  *WsHub
	conn *websocket.Conn
	send chan storage.Event
}

func (c *WsClient) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for event := range c.send {
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		err := c.conn.WriteJSON(event)
		if err != nil {
			c.hub.logger.Error("websocket write error", "error", err)
			return
		}
	}
}

type WsHub struct {
	logger     *slog.Logger
	clients    map[*WsClient]bool
	broadcast  chan storage.Event
	register   chan *WsClient
	unregister chan *WsClient
	mu         sync.Mutex
}

func NewWsHub(logger *slog.Logger) *WsHub {
	return &WsHub{
		logger:     logger,
		clients:    make(map[*WsClient]bool),
		broadcast:  make(chan storage.Event, sendBufferSize),
		register:   make(chan *WsClient),
		unregister: make(chan *WsClient),
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
				close(client.send)
			}
			h.mu.Unlock()
		case event := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- event:
				default:
					h.logger.Warn("websocket client buffer full, dropping client")
					close(client.send)
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

	if len(api.authorizedKeys) > 0 {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		var authMsg struct {
			ApiKey string `json:"api_key"`
		}
		err = conn.ReadJSON(&authMsg)
		if _, ok := api.authorizedKeys[authMsg.ApiKey]; err != nil || !ok {
			api.logger.Warn("websocket auth failed", "error", err)
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Unauthorized"))
			conn.Close()
			return
		}
		conn.SetReadDeadline(time.Time{})
	}

	client := &WsClient{
		hub:  api.hub,
		conn: conn,
		send: make(chan storage.Event, sendBufferSize),
	}

	api.hub.register <- client
	go client.writePump()

	go func() {
		defer func() {
			api.hub.unregister <- client
			conn.Close()
		}()

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
	select {
	case h.broadcast <- event:
	default:
		h.logger.Warn("websocket broadcast channel full, dropping event")
	}
}
