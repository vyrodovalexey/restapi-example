package handler

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
)

// WebSocket configuration constants.
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
	sendInterval   = 1 * time.Second
)

// WebSocketHandler handles WebSocket connections.
type WebSocketHandler struct {
	upgrader websocket.Upgrader
	logger   *zap.Logger
	mu       sync.RWMutex
	clients  map[*websocket.Conn]context.CancelFunc
}

// NewWebSocketHandler creates a new WebSocketHandler instance.
func NewWebSocketHandler(logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		logger:  logger,
		clients: make(map[*websocket.Conn]context.CancelFunc),
	}
}

// RegisterRoutes registers the WebSocket routes with the router.
func (h *WebSocketHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/ws", h.HandleWebSocket).Methods(http.MethodGet)
}

// HandleWebSocket handles WebSocket connection requests.
//
//nolint:contextcheck // intentional: WebSocket connections outlive the HTTP request context
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("failed to upgrade connection", zap.Error(err))
		return
	}

	// Use background context instead of request context because the HTTP request
	// context gets canceled when the handler returns, but WebSocket connections
	// need to persist beyond the initial HTTP upgrade.
	ctx, cancel := context.WithCancel(context.Background())

	h.mu.Lock()
	h.clients[conn] = cancel
	h.mu.Unlock()

	h.logger.Info("websocket client connected", zap.String("remote_addr", conn.RemoteAddr().String()))

	go h.writePump(ctx, conn)
	go h.readPump(ctx, conn, cancel)
}

// readPump handles incoming messages from the WebSocket connection.
func (h *WebSocketHandler) readPump(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	defer func() {
		cancel()
		h.removeClient(conn)
		if err := conn.Close(); err != nil {
			h.logger.Debug("error closing connection", zap.Error(err))
		}
	}()

	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		h.logger.Error("failed to set read deadline", zap.Error(err))
		return
	}

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Warn("websocket read error", zap.Error(err))
				}
				return
			}
			h.logger.Debug("received message", zap.ByteString("message", message))
		}
	}
}

// writePump sends random values to the WebSocket connection every second.
func (h *WebSocketHandler) writePump(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(sendInterval)
	pingTicker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		pingTicker.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			h.sendCloseMessage(conn)
			return
		case <-ticker.C:
			if err := h.sendRandomValue(conn); err != nil {
				h.logger.Debug("failed to send random value", zap.Error(err))
				return
			}
		case <-pingTicker.C:
			if err := h.sendPing(conn); err != nil {
				h.logger.Debug("failed to send ping", zap.Error(err))
				return
			}
		}
	}
}

// sendRandomValue sends a random value message to the connection.
func (h *WebSocketHandler) sendRandomValue(conn *websocket.Conn) error {
	value, err := generateSecureRandomInt()
	if err != nil {
		h.logger.Error("failed to generate random value", zap.Error(err))
		return err
	}

	msg := model.NewRandomValueMessage(value)

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}

	return conn.WriteJSON(msg)
}

// sendPing sends a ping message to the connection.
func (h *WebSocketHandler) sendPing(conn *websocket.Conn) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.PingMessage, nil)
}

// sendCloseMessage sends a close message to the connection.
func (h *WebSocketHandler) sendCloseMessage(conn *websocket.Conn) {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		h.logger.Debug("failed to set write deadline for close", zap.Error(err))
		return
	}

	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down")
	if err := conn.WriteMessage(websocket.CloseMessage, closeMsg); err != nil {
		h.logger.Debug("failed to send close message", zap.Error(err))
	}
}

// removeClient removes a client from the clients map.
func (h *WebSocketHandler) removeClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if cancel, exists := h.clients[conn]; exists {
		cancel()
		delete(h.clients, conn)
		h.logger.Info("websocket client disconnected", zap.String("remote_addr", conn.RemoteAddr().String()))
	}
}

// CloseAllConnections closes all active WebSocket connections.
func (h *WebSocketHandler) CloseAllConnections() {
	h.mu.Lock()
	// Copy the clients map to avoid holding the lock while closing
	clients := make(map[*websocket.Conn]context.CancelFunc, len(h.clients))
	for conn, cancel := range h.clients {
		clients[conn] = cancel
	}
	h.mu.Unlock()

	// Cancel all contexts first - this will trigger writePump to send close messages
	for _, cancel := range clients {
		cancel()
	}

	// Give writePump goroutines time to send close messages
	time.Sleep(100 * time.Millisecond)

	// Now close all connections
	h.mu.Lock()
	for conn := range h.clients {
		if err := conn.Close(); err != nil {
			h.logger.Debug("error closing connection", zap.Error(err))
		}
		delete(h.clients, conn)
	}
	h.mu.Unlock()

	h.logger.Info("all websocket connections closed")
}

// generateSecureRandomInt generates a cryptographically secure random integer.
func generateSecureRandomInt() (int, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	// Use uint32 to avoid integer overflow on 32-bit systems
	// Mask to ensure positive value
	return int(binary.BigEndian.Uint32(buf[:]) & 0x7FFFFFFF), nil
}
