package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dipjyotimetia/traffic-inspector/config"
	"github.com/gorilla/websocket"
)

type mockWebSocketServer struct {
	server    *httptest.Server
	upgrader  websocket.Upgrader
	messages  []string
	connected chan bool
	received  chan []byte
}

// Create a new mock WebSocket server for testing
func newMockWebSocketServer() *mockWebSocketServer {
	mock := &mockWebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		messages:  make([]string, 0),
		connected: make(chan bool, 1),
		received:  make(chan []byte, 10),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo server that returns messages back to the client
		conn, err := mock.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Signal that a client connected
		mock.connected <- true

		// Simple echo server
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// Record received message
			mock.messages = append(mock.messages, string(p))
			mock.received <- p

			// Send it back
			if err := conn.WriteMessage(messageType, p); err != nil {
				break
			}
		}
	}))

	return mock
}

func (m *mockWebSocketServer) Close() {
	m.server.Close()
}

func (m *mockWebSocketServer) URL() string {
	// Convert HTTP URL to WebSocket URL
	return "ws" + strings.TrimPrefix(m.server.URL, "http")
}

func setupTestDB(t *testing.T) (*sql.DB, *sql.Stmt, string) {
	// Create a temporary database file
	tempFile, err := os.CreateTemp("", "websocket_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database: %v", err)
	}
	tempFile.Close()
	dbPath := tempFile.Name()

	// Initialize database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create schema
	_, err = db.Exec(`
        CREATE TABLE traffic_records (
            id TEXT PRIMARY KEY,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            protocol TEXT NOT NULL,
            method TEXT NOT NULL, 
            url TEXT,
            service TEXT,
            request_headers TEXT,
            request_body BLOB,
            response_status INTEGER NOT NULL,
            response_headers TEXT,
            response_body BLOB,
            duration_ms INTEGER,
            client_ip TEXT,
            test_id TEXT,
            session_id TEXT,
            connection_id TEXT,
            message_type INTEGER,
            direction TEXT
        );

        CREATE INDEX idx_ws_lookup ON traffic_records(protocol, connection_id) WHERE protocol = 'WebSocket';
    `)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Prepare insert statement
	stmt, err := db.Prepare(`INSERT INTO traffic_records (
        id, timestamp, protocol, method, url, service,
        request_headers, request_body, response_status,
        response_headers, response_body, duration_ms,
        client_ip, test_id, session_id, connection_id,
        message_type, direction
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}

	return db, stmt, dbPath
}

func TestWebSocketProxy_PassthroughMode(t *testing.T) {
	// Start mock WebSocket server
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	// Setup test database
	db, stmt, dbPath := setupTestDB(t)
	defer db.Close()
	defer stmt.Close()
	defer os.Remove(dbPath)

	// Create configuration
	cfg := &config.Config{
		HTTPPort:      8888, // Doesn't matter for test
		HTTPTargetURL: mockServer.server.URL,
		RecordingMode: false,
		ReplayMode:    false,
		WebSocket: config.WebSocketConfig{
			Enabled:         true,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	// Create WebSocket proxy
	wsProxy := NewWebSocketProxy(cfg, db, stmt)

	// Create test HTTP server that uses our WebSocket proxy
	proxyServer := httptest.NewServer(wsProxy)
	defer proxyServer.Close()

	// Create WebSocket client
	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/ws"
	dialer := websocket.DefaultDialer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to proxy WebSocket
	clientConn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket proxy: %v", err)
	}
	defer clientConn.Close()

	// Wait for connection to be established with mock server
	select {
	case <-mockServer.connected:
		// Connection successful
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for WebSocket connection")
	}

	// Send test message
	testMessage := []byte("Hello, WebSocket proxy!")
	err = clientConn.WriteMessage(websocket.TextMessage, testMessage)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for message to be received by mock server
	var receivedByServer []byte
	select {
	case receivedByServer = <-mockServer.received:
		// Message received
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message to be received by server")
	}

	if string(receivedByServer) != string(testMessage) {
		t.Errorf("Server received incorrect message. Got %q, want %q",
			string(receivedByServer), string(testMessage))
	}

	// Check for echo reply
	messageType, receivedMessage, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive response: %v", err)
	}

	if messageType != websocket.TextMessage {
		t.Errorf("Unexpected message type: got %d, want %d", messageType, websocket.TextMessage)
	}

	if string(receivedMessage) != string(testMessage) {
		t.Errorf("Received incorrect echo. Got %q, want %q",
			string(receivedMessage), string(testMessage))
	}
}

func TestWebSocketProxy_RecordMode(t *testing.T) {
	// Start mock WebSocket server
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	// Setup test database
	db, stmt, dbPath := setupTestDB(t)
	defer db.Close()
	defer stmt.Close()
	defer os.Remove(dbPath)

	// Create configuration with recording mode enabled
	cfg := &config.Config{
		HTTPPort:      8889,
		HTTPTargetURL: mockServer.server.URL,
		RecordingMode: true, // Enable recording
		ReplayMode:    false,
		WebSocket: config.WebSocketConfig{
			Enabled:         true,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	// Create WebSocket proxy
	wsProxy := NewWebSocketProxy(cfg, db, stmt)

	// Create test HTTP server that uses our WebSocket proxy
	proxyServer := httptest.NewServer(wsProxy)
	defer proxyServer.Close()

	// Create WebSocket client
	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/ws"
	dialer := websocket.DefaultDialer

	// Connect to proxy WebSocket
	clientConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket proxy: %v", err)
	}
	defer clientConn.Close()

	// Wait for connection to be established
	select {
	case <-mockServer.connected:
		// Connection successful
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for WebSocket connection")
	}

	// Send test message
	testMessage := []byte(`{"action": "test", "data": "message"}`)
	err = clientConn.WriteMessage(websocket.TextMessage, testMessage)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for response
	_, response, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive response: %v", err)
	}

	// Close connection to ensure all records are committed
	clientConn.Close()
	time.Sleep(100 * time.Millisecond) // Give some time for async recording to complete

	// Check database for recording
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM traffic_records WHERE protocol = 'WebSocket'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query WebSocket records: %v", err)
	}

	// We expect at least 3 records:
	// 1. Connection establishment (CONNECT)
	// 2. Client message (MESSAGE outbound)
	// 3. Server response (MESSAGE inbound)
	// 4. Optionally: Connection close (CLOSE) - may be async
	if count < 3 {
		t.Errorf("Expected at least 3 WebSocket records, got %d", count)
	}

	// Check message content in database
	var msgBody []byte
	err = db.QueryRow(
		"SELECT request_body FROM traffic_records WHERE protocol = 'WebSocket' AND method = 'MESSAGE' AND direction = 'outbound' LIMIT 1",
	).Scan(&msgBody)
	if err != nil {
		t.Fatalf("Failed to query WebSocket message: %v", err)
	}

	if string(msgBody) != string(testMessage) {
		t.Errorf("Recorded message doesn't match. Got %q, want %q", string(msgBody), string(testMessage))
	}

	// Check response in database
	var respBody []byte
	err = db.QueryRow(
		"SELECT response_body FROM traffic_records WHERE protocol = 'WebSocket' AND method = 'MESSAGE' AND direction = 'inbound' LIMIT 1",
	).Scan(&respBody)
	if err != nil {
		t.Fatalf("Failed to query WebSocket response: %v", err)
	}

	if string(respBody) != string(response) {
		t.Errorf("Recorded response doesn't match. Got %q, want %q", string(respBody), string(response))
	}
}

func TestWebSocketProxy_ReplayMode(t *testing.T) {
	// Setup test database with pre-recorded data
	db, stmt, dbPath := setupTestDB(t)
	defer db.Close()
	defer stmt.Close()
	defer os.Remove(dbPath)

	// Insert pre-recorded WebSocket messages
	testURL := "/ws/test"
	connectionID := "test-connection-123"

	// Insert a series of messages to replay
	messages := []struct {
		id          string
		method      string
		messageType int
		body        []byte
		direction   string
		timestamp   time.Time
	}{
		{
			id:          "msg-1",
			method:      "CONNECT",
			messageType: 0,
			body:        nil,
			direction:   "",
			timestamp:   time.Now().Add(-5 * time.Second),
		},
		{
			id:          "msg-2",
			method:      "MESSAGE",
			messageType: websocket.TextMessage,
			body:        []byte(`{"action": "welcome", "message": "Hello, client!"}`),
			direction:   "inbound",
			timestamp:   time.Now().Add(-4 * time.Second),
		},
		{
			id:          "msg-3",
			method:      "MESSAGE",
			messageType: websocket.TextMessage,
			body:        []byte(`{"action": "update", "data": {"value": 42}}`),
			direction:   "inbound",
			timestamp:   time.Now().Add(-2 * time.Second),
		},
		{
			id:          "msg-4",
			method:      "CLOSE",
			messageType: 0,
			body:        nil,
			direction:   "",
			timestamp:   time.Now().Add(-1 * time.Second),
		},
	}

	for _, msg := range messages {
		_, err := stmt.Exec(
			msg.id,
			msg.timestamp,
			"WebSocket",
			msg.method,
			testURL,
			"",  // service
			"",  // requestHeaders
			nil, // requestBody
			101, // responseStatus
			"",  // responseHeaders
			msg.body,
			0, // duration
			"127.0.0.1",
			"test-id",
			"test-session",
			connectionID,
			msg.messageType,
			msg.direction,
		)
		if err != nil {
			t.Fatalf("Failed to insert test message: %v", err)
		}
	}

	// Create configuration with replay mode enabled
	cfg := &config.Config{
		HTTPPort:      8890,
		HTTPTargetURL: "http://example.com", // Won't be used in replay mode
		RecordingMode: false,
		ReplayMode:    true, // Enable replay
		WebSocket: config.WebSocketConfig{
			Enabled:         true,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	// Create WebSocket proxy in replay mode
	wsProxy := NewWebSocketProxy(cfg, db, stmt)

	// Create test HTTP server that uses our WebSocket proxy
	proxyServer := httptest.NewServer(wsProxy)
	defer proxyServer.Close()

	// Create WebSocket client
	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + testURL
	dialer := websocket.DefaultDialer

	// Connect to proxy WebSocket
	clientConn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket proxy in replay mode: %v", err)
	}
	defer clientConn.Close()

	// We should receive the first message
	_, msg1, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read first replayed message: %v", err)
	}

	// Verify the first message content
	var msg1Data map[string]interface{}
	if err := json.Unmarshal(msg1, &msg1Data); err != nil {
		t.Fatalf("Failed to parse first message: %v", err)
	}

	if msg1Data["action"] != "welcome" {
		t.Errorf("First message has incorrect action: got %v, want %s",
			msg1Data["action"], "welcome")
	}

	// We should receive the second message
	_, msg2, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read second replayed message: %v", err)
	}

	// Verify the second message content
	var msg2Data map[string]interface{}
	if err := json.Unmarshal(msg2, &msg2Data); err != nil {
		t.Fatalf("Failed to parse second message: %v", err)
	}

	if msg2Data["action"] != "update" {
		t.Errorf("Second message has incorrect action: got %v, want %s",
			msg2Data["action"], "update")
	}

	// Send a message (which should be ignored in replay mode)
	clientConn.WriteMessage(websocket.TextMessage, []byte("This will be ignored"))
}

func TestWebSocketProxy_ErrorHandling(t *testing.T) {
	// Setup test database
	db, stmt, dbPath := setupTestDB(t)
	defer db.Close()
	defer stmt.Close()
	defer os.Remove(dbPath)

	// Create configuration with an unreachable target
	unreachableHost := "ws://non-existent-host-for-testing:12345"
	cfg := &config.Config{
		HTTPPort:      8891,
		HTTPTargetURL: unreachableHost,
		RecordingMode: false,
		ReplayMode:    false,
		WebSocket: config.WebSocketConfig{
			Enabled:         true,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	// Create WebSocket proxy
	wsProxy := NewWebSocketProxy(cfg, db, stmt)

	// Create test HTTP server that uses our WebSocket proxy
	proxyServer := httptest.NewServer(wsProxy)
	defer proxyServer.Close()

	// Create WebSocket client
	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/ws"
	dialer := websocket.DefaultDialer

	// Connect to proxy WebSocket - should fail because target is unreachable
	_, resp, err := dialer.Dial(wsURL, nil)

	// Check that we got an error response
	if err == nil {
		t.Fatal("Expected WebSocket connection to fail, but it succeeded")
	}

	if resp == nil {
		t.Fatal("Expected HTTP response when connection fails, but got nil")
	}

	// Should return 502 Bad Gateway or similar
	if resp.StatusCode < 400 {
		t.Errorf("Expected error status code, got %d", resp.StatusCode)
	}
}
