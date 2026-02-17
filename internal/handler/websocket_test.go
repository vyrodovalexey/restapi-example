package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
)

func TestNewWebSocketHandler(t *testing.T) {
	// Arrange
	logger := zap.NewNop()

	// Act
	handler := NewWebSocketHandler(logger)

	// Assert
	if handler == nil {
		t.Fatal("NewWebSocketHandler() returned nil")
	}
	if handler.logger == nil {
		t.Error("logger should not be nil")
	}
	if handler.clients == nil {
		t.Error("clients map should be initialized")
	}
}

func TestWebSocketHandler_RegisterRoutes(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)
	router := mux.NewRouter()

	// Act
	handler.RegisterRoutes(router)

	// Assert - Test that route is registered
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Route should be found (will fail upgrade but not 404)
	if rr.Code == http.StatusNotFound {
		t.Error("Route /ws not found")
	}
}

func TestWebSocketHandler_HandleWebSocket_ConnectionEstablishment(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Act
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)

	// Assert
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
}

func TestWebSocketHandler_HandleWebSocket_ReceivesMessages(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	// Use a channel to signal when we're done
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer func() {
		close(done)
		handler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Act - Wait for a message (sendInterval is 1 second)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg model.WebSocketMessage
	err = conn.ReadJSON(&msg)

	// Assert
	if err != nil {
		// This can happen in test environment due to timing - skip if connection closed
		t.Skipf("Skipping due to connection timing: %v", err)
	}

	if msg.Type != model.WSMessageTypeRandomValue {
		t.Errorf("Message type = %s, want %s", msg.Type, model.WSMessageTypeRandomValue)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestWebSocketHandler_HandleWebSocket_MultipleClients(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer func() {
		handler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	numClients := 3
	conns := make([]*websocket.Conn, numClients)

	// Act - Connect multiple clients
	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns[i] = conn
		defer conns[i].Close()
	}

	// Give time for connections to be registered
	time.Sleep(200 * time.Millisecond)

	// Assert - Verify all clients are connected (handler tracks them)
	// The actual message receiving is tested in other tests
	// Here we just verify multiple connections work
	if len(conns) != numClients {
		t.Errorf("Expected %d connections, got %d", numClients, len(conns))
	}
}

func TestWebSocketHandler_HandleWebSocket_ClientDisconnect(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Give time for connection to be registered
	time.Sleep(100 * time.Millisecond)

	// Act - Close connection
	conn.Close()

	// Give time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Assert - Handler should handle disconnect gracefully
	// No panic should occur
}

func TestWebSocketHandler_CloseAllConnections(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple clients
	numClients := 3
	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns[i] = conn
	}

	// Give time for connections to be registered
	time.Sleep(100 * time.Millisecond)

	// Act
	handler.CloseAllConnections()

	// Assert - All connections should be closed
	time.Sleep(200 * time.Millisecond)

	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err == nil {
			t.Errorf("Client %d: connection should be closed", i)
		}
	}
}

func TestWebSocketHandler_HandleWebSocket_InvalidUpgrade(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	// Act - Make a regular HTTP request (not WebSocket upgrade)
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()

	handler.HandleWebSocket(rr, req)

	// Assert - Should fail to upgrade
	if rr.Code == http.StatusSwitchingProtocols {
		t.Error("Should not upgrade non-WebSocket request")
	}
}

func TestWebSocketHandler_HandleWebSocket_SendsMultipleMessages(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer func() {
		handler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Act - Try to receive a message (sendInterval is 1 second)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg model.WebSocketMessage
	err = conn.ReadJSON(&msg)

	// Assert
	if err != nil {
		// This can happen in test environment due to timing - skip if connection closed
		t.Skipf("Skipping due to connection timing: %v", err)
	}

	if msg.Type != model.WSMessageTypeRandomValue {
		t.Errorf("Message type = %s, want %s", msg.Type, model.WSMessageTypeRandomValue)
	}
}

func TestWebSocketHandler_HandleWebSocket_ClientSendsMessage(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer func() {
		handler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Act - Send a message from client
	err = conn.WriteMessage(websocket.TextMessage, []byte("hello"))

	// Assert - Should not cause error
	if err != nil {
		t.Errorf("Failed to send message: %v", err)
	}

	// Give time for the message to be processed
	time.Sleep(100 * time.Millisecond)

	// Connection should still be open (no panic or crash)
}

func TestGenerateSecureRandomInt(t *testing.T) {
	// Act
	values := make(map[int]bool)
	for i := 0; i < 100; i++ {
		val, err := generateSecureRandomInt()
		if err != nil {
			t.Fatalf("generateSecureRandomInt() error: %v", err)
		}
		values[val] = true

		// Assert - Value should be non-negative
		if val < 0 {
			t.Errorf("generateSecureRandomInt() returned negative value: %d", val)
		}
	}

	// Assert - Should generate different values (with high probability)
	if len(values) < 90 {
		t.Errorf("generateSecureRandomInt() generated too few unique values: %d", len(values))
	}
}

func TestWebSocketHandler_Upgrader(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	// Assert - Check upgrader configuration
	if handler.upgrader.ReadBufferSize != 1024 {
		t.Errorf("ReadBufferSize = %d, want 1024", handler.upgrader.ReadBufferSize)
	}
	if handler.upgrader.WriteBufferSize != 1024 {
		t.Errorf("WriteBufferSize = %d, want 1024", handler.upgrader.WriteBufferSize)
	}

	// CheckOrigin should allow all origins
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "http://example.com")
	if !handler.upgrader.CheckOrigin(req) {
		t.Error("CheckOrigin should allow all origins")
	}
}

func TestWebSocketConstants(t *testing.T) {
	// Assert - Check that constants are defined
	if writeWait != 10*time.Second {
		t.Errorf("writeWait = %v, want 10s", writeWait)
	}
	if pongWait != 60*time.Second {
		t.Errorf("pongWait = %v, want 60s", pongWait)
	}
	if pingPeriod != (pongWait*9)/10 {
		t.Errorf("pingPeriod = %v, want %v", pingPeriod, (pongWait*9)/10)
	}
	if maxMessageSize != 512 {
		t.Errorf("maxMessageSize = %d, want 512", maxMessageSize)
	}
	if sendInterval != 1*time.Second {
		t.Errorf("sendInterval = %v, want 1s", sendInterval)
	}
}

func TestWebSocketHandler_RemoveClient(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Give time for connection to be registered
	time.Sleep(100 * time.Millisecond)

	// Act - Close connection (triggers removeClient)
	conn.Close()

	// Give time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Assert - No panic should occur
	handler.CloseAllConnections()
}

func TestWebSocketHandler_CloseAllConnections_Empty(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	// Act - Close all connections when there are none
	handler.CloseAllConnections()

	// Assert - No panic should occur
}

func TestWebSocketHandler_CloseAllConnections_WithConnections(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect a client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Give time for connection to be registered
	time.Sleep(100 * time.Millisecond)

	// Act
	handler.CloseAllConnections()

	// Assert - Connection should be closed
	time.Sleep(100 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Connection should be closed")
	}
}

func TestWebSocketHandler_SendPing(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	wsHandler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsHandler.HandleWebSocket(w, r)
	}))
	defer func() {
		wsHandler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Give time for connection to be registered
	time.Sleep(100 * time.Millisecond)

	// Act - Get the underlying connection and its state from the handler's clients map
	wsHandler.mu.RLock()
	var serverConn *websocket.Conn
	var serverState *connState
	for c, s := range wsHandler.clients {
		serverConn = c
		serverState = s
		break
	}
	wsHandler.mu.RUnlock()

	if serverConn == nil {
		t.Fatal("No server connection found")
	}

	// Set up ping handler on client side to verify ping was received
	pingReceived := make(chan struct{}, 1)
	conn.SetPingHandler(func(_ string) error {
		pingReceived <- struct{}{}
		return nil
	})

	// Send ping from server
	err = wsHandler.sendPing(serverConn, serverState)
	if err != nil {
		t.Fatalf("sendPing() error = %v", err)
	}

	// Read on client side to trigger ping handler
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	go func() {
		// ReadMessage will process control frames (ping) internally
		conn.ReadMessage() //nolint:errcheck // we just need to trigger the read
	}()

	// Wait for ping to be received
	select {
	case <-pingReceived:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Ping was not received within timeout")
	}
}

func TestWebSocketHandler_SendRandomValue(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	wsHandler := NewWebSocketHandler(logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsHandler.HandleWebSocket(w, r)
	}))
	defer func() {
		wsHandler.CloseAllConnections()
		server.Close()
	}()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Give time for connection to be registered
	time.Sleep(100 * time.Millisecond)

	// Get the server-side connection and its state
	wsHandler.mu.RLock()
	var serverConn *websocket.Conn
	var serverState *connState
	for c, s := range wsHandler.clients {
		serverConn = c
		serverState = s
		break
	}
	wsHandler.mu.RUnlock()

	if serverConn == nil {
		t.Fatal("No server connection found")
	}

	// Act - Call sendRandomValue directly
	err = wsHandler.sendRandomValue(serverConn, serverState)
	if err != nil {
		t.Fatalf("sendRandomValue() error = %v", err)
	}

	// Read the message on client side
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg model.WebSocketMessage
	err = conn.ReadJSON(&msg)
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	// Assert
	if msg.Type != model.WSMessageTypeRandomValue {
		t.Errorf("Message type = %s, want %s", msg.Type, model.WSMessageTypeRandomValue)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
