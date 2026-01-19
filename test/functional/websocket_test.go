//go:build functional

package functional

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketMessage represents a message received from WebSocket.
type WebSocketMessage struct {
	Type      string    `json:"type"`
	Value     int       `json:"value,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// WebSocketClient wraps a WebSocket connection for testing.
type WebSocketClient struct {
	conn *websocket.Conn
	t    *testing.T
}

// NewWebSocketClient creates a new WebSocket client connected to the given URL.
func NewWebSocketClient(t *testing.T, url string) (*WebSocketClient, error) {
	t.Helper()

	dialer := websocket.Dialer{
		HandshakeTimeout: DefaultWebSocketTimeout,
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	return &WebSocketClient{
		conn: conn,
		t:    t,
	}, nil
}

// ReadMessage reads a single message from the WebSocket.
func (c *WebSocketClient) ReadMessage(timeout time.Duration) (*WebSocketMessage, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))

	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// ReadMessages reads multiple messages from the WebSocket.
func (c *WebSocketClient) ReadMessages(count int, timeout time.Duration) ([]*WebSocketMessage, error) {
	messages := make([]*WebSocketMessage, 0, count)
	deadline := time.Now().Add(timeout)

	for len(messages) < count {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		msg, err := c.ReadMessage(remaining)
		if err != nil {
			return messages, err
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// Close closes the WebSocket connection.
func (c *WebSocketClient) Close() error {
	return c.conn.Close()
}

// CloseGracefully sends a close message and waits for acknowledgment.
func (c *WebSocketClient) CloseGracefully() error {
	// Send close message
	err := c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if err != nil {
		return err
	}

	// Wait briefly for close acknowledgment
	c.conn.SetReadDeadline(time.Now().Add(time.Second))
	c.conn.ReadMessage() // Ignore error, just drain

	return c.conn.Close()
}

// TestFunctional_WS_001_Connect tests WebSocket connection establishment.
// FT-WS-001: Connect to WebSocket (connection established)
func TestFunctional_WS_001_Connect(t *testing.T) {
	LogTestStart(t, "FT-WS-001", "Connect to WebSocket")
	defer LogTestEnd(t, "FT-WS-001")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	// Act
	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	// Assert - connection was established successfully
	t.Log("WebSocket connection established successfully")
}

// TestFunctional_WS_002_ReceiveRandomValue tests receiving a random value message.
// FT-WS-002: Receive random value (valid message format)
func TestFunctional_WS_002_ReceiveRandomValue(t *testing.T) {
	LogTestStart(t, "FT-WS-002", "Receive random value")
	defer LogTestEnd(t, "FT-WS-002")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	// Act - wait for a message
	msg, err := client.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	// Assert - validate message format
	if msg.Type != "random_value" {
		t.Errorf("Expected message type 'random_value', got %q", msg.Type)
	}

	if msg.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	// Value should be a non-negative integer (based on implementation)
	if msg.Value < 0 {
		t.Errorf("Expected non-negative value, got %d", msg.Value)
	}

	t.Logf("Received message: type=%s, value=%d, timestamp=%s", msg.Type, msg.Value, msg.Timestamp)
}

// TestFunctional_WS_003_ReceiveMultipleValues tests receiving multiple messages.
// FT-WS-003: Receive multiple values (5 messages in 7 seconds)
func TestFunctional_WS_003_ReceiveMultipleValues(t *testing.T) {
	LogTestStart(t, "FT-WS-003", "Receive multiple values")
	defer LogTestEnd(t, "FT-WS-003")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	const expectedMessages = 5
	const timeout = 7 * time.Second

	// Act
	messages, err := client.ReadMessages(expectedMessages, timeout)
	if err != nil {
		t.Logf("Read error (may be expected if timeout): %v", err)
	}

	// Assert
	if len(messages) < expectedMessages {
		t.Errorf("Expected at least %d messages, got %d", expectedMessages, len(messages))
	}

	for i, msg := range messages {
		t.Logf("Message %d: type=%s, value=%d", i+1, msg.Type, msg.Value)
	}
}

// TestFunctional_WS_004_ValuesAreRandom tests that received values are random.
// FT-WS-004: Values are random (not all same)
func TestFunctional_WS_004_ValuesAreRandom(t *testing.T) {
	LogTestStart(t, "FT-WS-004", "Values are random")
	defer LogTestEnd(t, "FT-WS-004")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	const messagesToCollect = 5
	const timeout = 7 * time.Second

	// Act
	messages, err := client.ReadMessages(messagesToCollect, timeout)
	if err != nil {
		t.Logf("Read error (may be expected if timeout): %v", err)
	}

	if len(messages) < 2 {
		t.Fatalf("Need at least 2 messages to check randomness, got %d", len(messages))
	}

	// Assert - check that not all values are the same
	values := make(map[int]bool)
	for _, msg := range messages {
		values[msg.Value] = true
	}

	if len(values) == 1 {
		t.Error("All received values are the same - values should be random")
	}

	t.Logf("Received %d unique values from %d messages", len(values), len(messages))
}

// TestFunctional_WS_005_MessageInterval tests message interval timing.
// FT-WS-005: Message interval ~1 second (900-1100ms)
func TestFunctional_WS_005_MessageInterval(t *testing.T) {
	LogTestStart(t, "FT-WS-005", "Message interval ~1 second")
	defer LogTestEnd(t, "FT-WS-005")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	const messagesToCollect = 3
	const timeout = 5 * time.Second
	const minInterval = 900 * time.Millisecond
	const maxInterval = 1100 * time.Millisecond

	// Collect messages with timestamps
	type timedMessage struct {
		msg        *WebSocketMessage
		receivedAt time.Time
	}

	messages := make([]timedMessage, 0, messagesToCollect)
	deadline := time.Now().Add(timeout)

	for len(messages) < messagesToCollect {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		msg, err := client.ReadMessage(remaining)
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		messages = append(messages, timedMessage{
			msg:        msg,
			receivedAt: time.Now(),
		})
	}

	if len(messages) < 2 {
		t.Fatalf("Need at least 2 messages to check interval, got %d", len(messages))
	}

	// Check intervals between consecutive messages
	for i := 1; i < len(messages); i++ {
		interval := messages[i].receivedAt.Sub(messages[i-1].receivedAt)
		t.Logf("Interval between message %d and %d: %v", i, i+1, interval)

		if interval < minInterval || interval > maxInterval {
			t.Errorf("Interval %v is outside expected range [%v, %v]", interval, minInterval, maxInterval)
		}
	}
}

// TestFunctional_WS_006_MultipleConcurrentClients tests multiple concurrent WebSocket clients.
// FT-WS-006: Multiple concurrent clients (5 clients receive messages)
func TestFunctional_WS_006_MultipleConcurrentClients(t *testing.T) {
	LogTestStart(t, "FT-WS-006", "Multiple concurrent clients")
	defer LogTestEnd(t, "FT-WS-006")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	const numClients = 5
	const timeout = 3 * time.Second

	var wg sync.WaitGroup
	results := make(chan bool, numClients)
	errors := make(chan error, numClients)

	// Launch concurrent clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
			if err != nil {
				errors <- err
				return
			}
			defer client.Close()

			// Try to receive at least one message
			msg, err := client.ReadMessage(timeout)
			if err != nil {
				errors <- err
				return
			}

			if msg.Type == "random_value" {
				results <- true
				t.Logf("Client %d received message: value=%d", clientID, msg.Value)
			} else {
				results <- false
			}
		}(i)
	}

	// Wait for all clients
	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Client error: %v", err)
	}

	// Count successful clients
	successCount := 0
	for success := range results {
		if success {
			successCount++
		}
	}

	if successCount != numClients {
		t.Errorf("Expected %d clients to receive messages, got %d", numClients, successCount)
	}
}

// TestFunctional_WS_007_ClientDisconnectHandling tests server handling of client disconnect.
// FT-WS-007: Client disconnect handling (server handles gracefully)
func TestFunctional_WS_007_ClientDisconnectHandling(t *testing.T) {
	LogTestStart(t, "FT-WS-007", "Client disconnect handling")
	defer LogTestEnd(t, "FT-WS-007")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	// Connect
	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	// Receive one message to confirm connection is working
	msg, err := client.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	t.Logf("Received message before disconnect: value=%d", msg.Value)

	// Disconnect abruptly
	err = client.Close()
	if err != nil {
		t.Logf("Close error (may be expected): %v", err)
	}

	// Give server time to handle disconnect
	time.Sleep(500 * time.Millisecond)

	// Verify server is still healthy
	httpClient := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	resp, err := httpClient.Get(ctx, "/health", nil)
	if err != nil {
		t.Fatalf("Health check failed after client disconnect: %v", err)
	}

	AssertStatusCode(t, resp, 200)
	t.Log("Server handled client disconnect gracefully")
}

// TestFunctional_WS_008_ReconnectionAfterDisconnect tests reconnection capability.
// FT-WS-008: Reconnection after disconnect
func TestFunctional_WS_008_ReconnectionAfterDisconnect(t *testing.T) {
	LogTestStart(t, "FT-WS-008", "Reconnection after disconnect")
	defer LogTestEnd(t, "FT-WS-008")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	// First connection
	t.Log("Establishing first connection")
	client1, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to establish first connection: %v", err)
	}

	// Receive a message
	msg1, err := client1.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("Failed to read message on first connection: %v", err)
	}
	t.Logf("First connection received: value=%d", msg1.Value)

	// Disconnect
	t.Log("Disconnecting first connection")
	client1.CloseGracefully()

	// Wait a moment
	time.Sleep(500 * time.Millisecond)

	// Reconnect
	t.Log("Establishing second connection (reconnect)")
	client2, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to reconnect: %v", err)
	}
	defer client2.Close()

	// Receive a message on new connection
	msg2, err := client2.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("Failed to read message on reconnection: %v", err)
	}
	t.Logf("Reconnection received: value=%d", msg2.Value)

	t.Log("Reconnection after disconnect successful")
}

// TestFunctional_WS_GracefulClose tests graceful WebSocket close.
func TestFunctional_WS_GracefulClose(t *testing.T) {
	LogTestStart(t, "FT-WS-EXTRA", "Graceful WebSocket close")
	defer LogTestEnd(t, "FT-WS-EXTRA")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	// Receive one message
	msg, err := client.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	t.Logf("Received message: value=%d", msg.Value)

	// Close gracefully
	err = client.CloseGracefully()
	if err != nil {
		t.Logf("Graceful close completed with: %v", err)
	}

	t.Log("Graceful close completed")
}

// TestFunctional_WS_MessageTimestampFormat tests that timestamps are in correct format.
func TestFunctional_WS_MessageTimestampFormat(t *testing.T) {
	LogTestStart(t, "FT-WS-EXTRA", "Message timestamp format")
	defer LogTestEnd(t, "FT-WS-EXTRA")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	// Read raw message
	client.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := client.conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	t.Logf("Raw message: %s", string(data))

	// Parse and verify timestamp
	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	// Timestamp should be recent (within last minute)
	now := time.Now()
	if msg.Timestamp.Before(now.Add(-time.Minute)) || msg.Timestamp.After(now.Add(time.Minute)) {
		t.Errorf("Timestamp %v is not recent", msg.Timestamp)
	}

	t.Logf("Timestamp is valid: %v", msg.Timestamp)
}

// TestFunctional_WS_LongRunningConnection tests a longer-running connection.
func TestFunctional_WS_LongRunningConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	LogTestStart(t, "FT-WS-EXTRA", "Long running connection")
	defer LogTestEnd(t, "FT-WS-EXTRA")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer client.Close()

	const duration = 10 * time.Second
	const expectedMinMessages = 8 // At least 8 messages in 10 seconds (1 per second)

	messages := make([]*WebSocketMessage, 0)
	deadline := time.Now().Add(duration)

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		msg, err := client.ReadMessage(remaining + time.Second)
		if err != nil {
			t.Logf("Read error: %v", err)
			break
		}

		messages = append(messages, msg)
	}

	if len(messages) < expectedMinMessages {
		t.Errorf("Expected at least %d messages in %v, got %d", expectedMinMessages, duration, len(messages))
	}

	t.Logf("Received %d messages over %v", len(messages), duration)
}
