package proxy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dipjyotimetia/traffic-inspector/config"
	"github.com/dipjyotimetia/traffic-inspector/internal/db"
	"github.com/gorilla/websocket"
)

// Upgrader for WebSocket connections
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections for proxying
	},
}

// Message types
const (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
	CloseMessage  = websocket.CloseMessage
)

// WebSocketProxy handles WebSocket connections
type WebSocketProxy struct {
	cfg        *config.Config
	database   *sql.DB
	insertStmt *sql.Stmt
}

// NewWebSocketProxy creates a new WebSocket proxy
func NewWebSocketProxy(cfg *config.Config, database *sql.DB, insertStmt *sql.Stmt) *WebSocketProxy {
	return &WebSocketProxy{
		cfg:        cfg,
		database:   database,
		insertStmt: insertStmt,
	}
}

// Generate a unique connection ID
func generateConnectionID() string {
	buf := make([]byte, 8)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

// ServeHTTP handles incoming WebSocket connections
func (p *WebSocketProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if it's a WebSocket request first
	if !isWebSocketRequest(r) {
		// Not a WebSocket request, let the HTTP handler deal with it
		return
	}

	// Generate a connection ID for this WebSocket session
	connectionID := generateConnectionID()
	log.Printf("🔌 WebSocket connection request: %s (ID: %s)", r.URL.String(), connectionID)

	if p.cfg.ReplayMode {
		p.handleReplayMode(w, r, connectionID)
	} else {
		p.handleProxyMode(w, r, connectionID)
	}
}

// Check if a request is a WebSocket upgrade request
func isWebSocketRequest(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

// Handle WebSocket in replay mode
// Handle WebSocket in replay mode
func (p *WebSocketProxy) handleReplayMode(w http.ResponseWriter, r *http.Request, connectionID string) {
	// Upgrade the connection first
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("🚨 Failed to upgrade WebSocket connection: %v", err)
		return
	}
	defer clientConn.Close()

	// Fetch and replay messages for this URL
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("🔁 Replaying WebSocket traffic for %s", r.URL.String())

	// Modified query to get both request_body and response_body for inbound/outbound messages
	// Only fetch MESSAGE method entries with valid message types
	rows, err := p.database.Query(`
        SELECT message_type, COALESCE(response_body, request_body) as body, timestamp 
        FROM traffic_records 
        WHERE protocol = 'WebSocket' AND url = ? AND method = 'MESSAGE' 
        AND message_type IN (?, ?)
        ORDER BY timestamp ASC`,
		r.URL.String(), TextMessage, BinaryMessage)
	if err != nil {
		log.Printf("🚨 DB error during WebSocket replay lookup: %v", err)
		clientConn.WriteMessage(TextMessage, []byte("Error retrieving WebSocket data"))
		return
	}
	defer rows.Close()

	var messages []struct {
		messageType int
		data        []byte
		timestamp   time.Time
	}

	for rows.Next() {
		var msg struct {
			messageType int
			data        []byte
			timestamp   time.Time
		}
		if err := rows.Scan(&msg.messageType, &msg.data, &msg.timestamp); err != nil {
			log.Printf("🚨 Error scanning WebSocket message: %v", err)
			continue
		}

		// Validate message type
		if msg.messageType != TextMessage && msg.messageType != BinaryMessage {
			log.Printf("⚠️ Skipping message with invalid type: %d", msg.messageType)
			continue
		}

		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		log.Printf("⚠️ No recorded WebSocket messages found for %s", r.URL.String())
		clientConn.WriteMessage(TextMessage, []byte("No recorded WebSocket data available"))
		clientConn.WriteMessage(CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "No messages to replay"))
		return
	}

	// Replay messages with timing
	var lastTime time.Time
	for i, msg := range messages {
		if i > 0 && !lastTime.IsZero() {
			// Calculate delay between messages
			delay := msg.timestamp.Sub(lastTime)
			if delay > 0 && delay < 10*time.Second {
				time.Sleep(delay)
			}
		}

		// Ensure message data is not nil before sending
		if msg.data == nil {
			log.Printf("⚠️ Skipping nil message data")
			continue
		}

		err := clientConn.WriteMessage(msg.messageType, msg.data)
		if err != nil {
			log.Printf("🚨 Error writing replayed message: %v", err)
			break
		}

		lastTime = msg.timestamp
		log.Printf("🔁 Replayed WebSocket message: type=%d, size=%d bytes",
			msg.messageType, len(msg.data))
	}

	// Send a proper close message to the client
	log.Printf("✅ Finished replaying all WebSocket messages, sending close message")
	clientConn.WriteMessage(CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Replay complete"))

	// Wait for client to close connection or timeout
	clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	for {
		_, _, err := clientConn.ReadMessage()
		if err != nil {
			log.Printf("📴 Client disconnected from replayed WebSocket: %v", err)
			break
		}
	}
}

// Handle WebSocket in proxy or record mode
func (p *WebSocketProxy) handleProxyMode(w http.ResponseWriter, r *http.Request, connectionID string) {
	// Determine target URL
	targetURLStr := p.cfg.GetTargetURL(r.URL.Path)
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		log.Printf("🚨 Invalid target URL %s: %v", targetURLStr, err)
		http.Error(w, "Invalid target URL", http.StatusBadGateway)
		return
	}

	// Convert to WebSocket URL
	wsScheme := "ws"
	if targetURL.Scheme == "https" {
		wsScheme = "wss"
	}
	targetURL.Scheme = wsScheme

	// Copy path and query from original request
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	// Connect to the target WebSocket server
	log.Printf("🔄 Connecting to target WebSocket: %s", targetURL.String())
	dialer := &websocket.Dialer{
		TLSClientConfig: p.cfg.GetTLSConfig(),
	}

	// Copy headers from client request
	requestHeader := http.Header{}
	for k, v := range r.Header {
		// Skip headers that would be set by the WebSocket handshake
		if k != "Sec-Websocket-Key" && k != "Sec-Websocket-Version" &&
			k != "Connection" && k != "Upgrade" &&
			k != "Sec-Websocket-Extensions" {
			requestHeader[k] = v
		}
	}

	// Add the original Host header
	requestHeader.Set("Host", r.Host)

	// Connect to target server
	targetConn, resp, err := dialer.Dial(targetURL.String(), requestHeader)
	if err != nil {
		log.Printf("🚨 Failed to connect to target WebSocket %s: %v", targetURL.String(), err)
		http.Error(w, fmt.Sprintf("Failed to connect to target: %v", err), http.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	// Record the handshake details if in recording mode
	if p.cfg.RecordingMode {
		log.Printf("📝 Recording WebSocket handshake: %s", r.URL.String())

		// Create record for handshake
		handshakeRecord := db.TrafficRecord{
			ID:             generateID(),
			Timestamp:      time.Now().UTC(),
			Protocol:       "WebSocket",
			Method:         "CONNECT",
			URL:            r.URL.String(),
			ConnectionID:   connectionID,
			ResponseStatus: resp.StatusCode,
			ClientIP:       getClientIP(r),
			SessionID:      r.Header.Get("X-Session-ID"),
			TestID:         r.Header.Get("X-Test-ID"),
		}

		if reqHeaderBytes, err := json.Marshal(r.Header); err == nil {
			handshakeRecord.RequestHeaders = string(reqHeaderBytes)
		}

		if respHeaderBytes, err := json.Marshal(resp.Header); err == nil {
			handshakeRecord.ResponseHeaders = string(respHeaderBytes)
		}

		err := saveTrafficRecord(handshakeRecord, p.insertStmt)
		if err != nil {
			log.Printf("⚠️ Failed to save WebSocket handshake: %v", err)
		}
	}

	// Upgrade the client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("🚨 Failed to upgrade client connection: %v", err)
		return
	}
	defer clientConn.Close()

	// Bidirectional copy of messages
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Target
	go func() {
		defer wg.Done()
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				log.Printf("📴 Client disconnected from WebSocket: %v", err)
				targetConn.Close()
				break
			}

			err = targetConn.WriteMessage(messageType, message)
			if err != nil {
				log.Printf("📴 Target disconnected from WebSocket: %v", err)
				clientConn.Close()
				break
			}

			if p.cfg.RecordingMode {
				// Record outgoing message
				record := db.TrafficRecord{
					ID:           generateID(),
					Timestamp:    time.Now().UTC(),
					Protocol:     "WebSocket",
					Method:       "MESSAGE",
					URL:          r.URL.String(),
					ConnectionID: connectionID,
					RequestBody:  message,
					MessageType:  messageType,
					Direction:    "outbound",
					ClientIP:     getClientIP(r),
					SessionID:    r.Header.Get("X-Session-ID"),
					TestID:       r.Header.Get("X-Test-ID"),
				}

				if err := saveTrafficRecord(record, p.insertStmt); err != nil {
					log.Printf("⚠️ Failed to save outbound WebSocket message: %v", err)
				}
			}
		}
	}()

	// Target -> Client
	go func() {
		defer wg.Done()
		for {
			messageType, message, err := targetConn.ReadMessage()
			if err != nil {
				log.Printf("📴 Target server closed WebSocket: %v", err)
				clientConn.Close()
				break
			}

			err = clientConn.WriteMessage(messageType, message)
			if err != nil {
				log.Printf("📴 Failed to forward to client: %v", err)
				targetConn.Close()
				break
			}

			if p.cfg.RecordingMode {
				// Record incoming message
				record := db.TrafficRecord{
					ID:           generateID(),
					Timestamp:    time.Now().UTC(),
					Protocol:     "WebSocket",
					Method:       "MESSAGE",
					URL:          r.URL.String(),
					ConnectionID: connectionID,
					ResponseBody: message,
					MessageType:  messageType,
					Direction:    "inbound",
					ClientIP:     getClientIP(r),
					SessionID:    r.Header.Get("X-Session-ID"),
					TestID:       r.Header.Get("X-Test-ID"),
				}

				if err := saveTrafficRecord(record, p.insertStmt); err != nil {
					log.Printf("⚠️ Failed to save inbound WebSocket message: %v", err)
				}
			}
		}
	}()

	// Wait for both goroutines to complete
	wg.Wait()
	log.Printf("📴 WebSocket connection closed: %s (ID: %s)", r.URL.String(), connectionID)

	// Record connection close if in recording mode
	if p.cfg.RecordingMode {
		closeRecord := db.TrafficRecord{
			ID:           generateID(),
			Timestamp:    time.Now().UTC(),
			Protocol:     "WebSocket",
			Method:       "CLOSE",
			URL:          r.URL.String(),
			ConnectionID: connectionID,
			ClientIP:     getClientIP(r),
			SessionID:    r.Header.Get("X-Session-ID"),
			TestID:       r.Header.Get("X-Test-ID"),
		}

		if err := saveTrafficRecord(closeRecord, p.insertStmt); err != nil {
			log.Printf("⚠️ Failed to save WebSocket close record: %v", err)
		}
	}
}
