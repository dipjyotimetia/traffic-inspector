package main

import "github.com/dipjyotimetia/traffic-inspector/cmd"

func main() {
	cmd.Execute()
}

// import (
// 	"bytes"
// 	"context"
// 	"crypto/rand"
// 	"database/sql"
// 	"encoding/hex"
// 	"encoding/json"
// 	"errors"
// 	"flag"
// 	"fmt"
// 	"io"
// 	"log"
// 	"net"
// 	"net/http"
// 	"net/http/httputil"
// 	"net/url"
// 	"os"
// 	"os/signal"
// 	"strings"
// 	"sync"
// 	"sync/atomic"
// 	"time"

// 	_ "github.com/mattn/go-sqlite3" // SQLite driver
// )

// // Configuration structure
// type Config struct {
// 	HTTPPort      int    `json:"http_port"`
// 	HTTPTargetURL string `json:"http_target_url"`
// 	SQLiteDBPath  string `json:"sqlite_db_path"`
// 	RecordingMode bool   `json:"recording_mode"`
// 	ReplayMode    bool   `json:"replay_mode"`
// }

// // TrafficRecord represents a captured API call
// type TrafficRecord struct {
// 	ID              string    `json:"id"`
// 	Timestamp       time.Time `json:"timestamp"`
// 	Protocol        string    `json:"protocol"` // HTTP only now
// 	Method          string    `json:"method"`   // HTTP method
// 	URL             string    `json:"url"`
// 	RequestHeaders  string    `json:"request_headers"`
// 	RequestBody     []byte    `json:"request_body"`
// 	ResponseStatus  int       `json:"response_status"` // HTTP status code
// 	ResponseHeaders string    `json:"response_headers"`
// 	ResponseBody    []byte    `json:"response_body"`
// 	Duration        int64     `json:"duration_ms"` // Duration in milliseconds
// 	ClientIP        string    `json:"client_ip"`
// 	TestID          string    `json:"test_id"`    // Optional: Link to test case ID
// 	SessionID       string    `json:"session_id"` // For grouping related requests
// }

// var (
// 	configFile = flag.String("config", "config.json", "Path to configuration file")
// 	config     Config
// 	db         *sql.DB
// 	insertStmt *sql.Stmt

// 	// Buffer pool for reusing byte slices
// 	bufferPool = sync.Pool{
// 		New: func() any {
// 			// Start with 4KB, can grow
// 			b := make([]byte, 0, 4*1024)
// 			return &b // Store pointers to slices
// 		},
// 	}
// )

// // Helper to get a buffer from the pool
// func getBuffer() *bytes.Buffer {
// 	buf := bufferPool.Get().(*[]byte)
// 	*buf = (*buf)[:0] // Reset slice length, keep capacity
// 	return bytes.NewBuffer(*buf)
// }

// // Helper to put a buffer back into the pool
// func putBuffer(buf *bytes.Buffer) {
// 	// If the buffer didn't grow excessively large, put it back
// 	// This prevents keeping very large buffers in the pool indefinitely
// 	const maxBufferSize = 1 * 1024 * 1024 // 1MB limit
// 	if buf != nil && buf.Cap() <= maxBufferSize {
// 		b := buf.Bytes()
// 		bufferPool.Put(&b) // Store pointer to the underlying slice
// 	}
// }

// func main() {
// 	flag.Parse()

// 	// Load configuration
// 	if err := loadConfig(*configFile); err != nil {
// 		log.Fatalf("❌ Failed to load configuration: %v", err)
// 	}
// 	log.Printf("🔧 Configuration loaded: Mode=%s", getMode())

// 	// Validate configuration
// 	if err := validateConfig(); err != nil {
// 		log.Fatalf("❌ Invalid configuration: %v", err)
// 	}

// 	// Initialize SQLite client
// 	var err error
// 	db, err = sql.Open("sqlite3", config.SQLiteDBPath+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000") // Use WAL and normal sync for better concurrency
// 	if err != nil {
// 		log.Fatalf("❌ Failed to open SQLite database: %v", err)
// 	}
// 	defer db.Close()

// 	// Set SQLite connection pool settings
// 	db.SetMaxOpenConns(25)
// 	db.SetMaxIdleConns(10) // Increased idle conns
// 	db.SetConnMaxLifetime(5 * time.Minute)

// 	if err := db.Ping(); err != nil {
// 		log.Fatalf("❌ Failed to ping SQLite database: %v", err)
// 	}
// 	log.Printf("🔗 Connected to SQLite database at %s", config.SQLiteDBPath)

// 	// Create schema and prepare statement
// 	if err := setupDatabase(); err != nil {
// 		log.Fatalf("❌ Failed to setup database: %v", err)
// 	}
// 	defer insertStmt.Close() // Ensure prepared statement is closed

// 	// Set up signal handling for graceful shutdown
// 	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
// 	defer cancel()

// 	var wg sync.WaitGroup

// 	// Start HTTP proxy server
// 	var httpServer *http.Server
// 	if config.HTTPPort > 0 {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			httpServer = startHTTPProxy(ctx) // Pass context for shutdown
// 		}()
// 	}

// 	// Wait for shutdown signal
// 	<-ctx.Done()
// 	log.Println("🚨 Shutdown signal received, initiating graceful shutdown...")

// 	// Graceful shutdown with timeout
// 	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second) // Increased timeout
// 	defer shutdownCancel()

// 	// Shutdown HTTP server
// 	if httpServer != nil {
// 		log.Println("⏳ Shutting down HTTP server...")
// 		if err := httpServer.Shutdown(shutdownCtx); err != nil {
// 			log.Printf("⚠️ HTTP server shutdown error: %v", err)
// 		} else {
// 			log.Println("✅ HTTP server stopped gracefully")
// 		}
// 	}

// 	// Wait for server goroutines to finish completely
// 	wg.Wait()
// 	log.Println("🏁 All servers stopped")
// }

// func getMode() string {
// 	if config.RecordingMode {
// 		return "Recording"
// 	}
// 	if config.ReplayMode {
// 		return "Replay"
// 	}
// 	return "Passthrough"
// }

// func loadConfig(path string) error {
// 	data, err := os.ReadFile(path)
// 	if err != nil {
// 		return fmt.Errorf("reading config file %s: %w", path, err)
// 	}

// 	if err := json.Unmarshal(data, &config); err != nil {
// 		return fmt.Errorf("parsing config file %s: %w", path, err)
// 	}

// 	// Set default SQLite path if not provided
// 	if config.SQLiteDBPath == "" {
// 		config.SQLiteDBPath = "traffic_inspector.db"
// 	}

// 	if config.RecordingMode && config.ReplayMode {
// 		return errors.New("cannot enable both RecordingMode and ReplayMode simultaneously")
// 	}

// 	return nil
// }

// func validateConfig() error {
// 	if config.HTTPPort <= 0 {
// 		return errors.New("http_port must be configured")
// 	}
// 	if config.HTTPTargetURL == "" {
// 		return errors.New("http_target_url must be set")
// 	}
// 	return nil
// }

// // HTTP Proxy implementation
// func startHTTPProxy(ctx context.Context) *http.Server {
// 	target, err := url.Parse(config.HTTPTargetURL)
// 	if err != nil {
// 		// This should be caught by validateConfig, but double-check
// 		log.Printf("🚨 ERROR: Invalid HTTP target URL (should not happen here): %v", err)
// 		return nil // Cannot start server
// 	}

// 	proxy := httputil.NewSingleHostReverseProxy(target)
// 	originalDirector := proxy.Director
// 	proxy.Director = func(req *http.Request) {
// 		originalDirector(req)
// 		req.Host = target.Host // Important for some backends
// 		// Clear X-Forwarded-For to prevent backend confusion if it relies on direct client IP
// 		// Or append client IP depending on requirements
// 		req.Header.Del("X-Forwarded-For")
// 		// Ensure User-Agent is passed if needed
// 		// req.Header.Set("User-Agent", r.UserAgent())
// 	}

// 	// Configure a more robust transport
// 	proxy.Transport = &http.Transport{
// 		Proxy: http.ProxyFromEnvironment,
// 		DialContext: (&net.Dialer{
// 			Timeout:   10 * time.Second, // Faster connection timeout
// 			KeepAlive: 60 * time.Second, // Longer keep-alive
// 		}).DialContext,
// 		ForceAttemptHTTP2:     true,
// 		MaxIdleConns:          200,              // More idle connections
// 		MaxIdleConnsPerHost:   100,              // Limit per target host
// 		IdleConnTimeout:       90 * time.Second, // Standard idle timeout
// 		TLSHandshakeTimeout:   10 * time.Second,
// 		ExpectContinueTimeout: 1 * time.Second,
// 		ResponseHeaderTimeout: 20 * time.Second, // Timeout for reading response headers
// 	}

// 	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
// 		log.Printf("🚨 HTTP proxy error: %v", err)
// 		rw.WriteHeader(http.StatusBadGateway)
// 	}

// 	// Buffer pool for the response writer wrapper
// 	responseBufPool := sync.Pool{
// 		New: func() interface{} {
// 			return new(bytes.Buffer)
// 		},
// 	}

// 	// Create the HTTP server
// 	server := &http.Server{
// 		Addr:    fmt.Sprintf(":%d", config.HTTPPort),
// 		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handleHTTPRequest(w, r, proxy, &responseBufPool) }),
// 		// Set timeouts
// 		ReadTimeout:  15 * time.Second,
// 		WriteTimeout: 30 * time.Second,
// 		IdleTimeout:  60 * time.Second,
// 	}

// 	log.Printf("🚀 Starting HTTP proxy server on port %d, proxying to %s", config.HTTPPort, config.HTTPTargetURL)
// 	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
// 		log.Printf("🚨 HTTP server error: %v", err)
// 	}

// 	return server // Return server instance for graceful shutdown
// }

// // handleHTTPRequest contains the logic for processing each HTTP request
// func handleHTTPRequest(w http.ResponseWriter, r *http.Request, proxy *httputil.ReverseProxy, responseBufPool *sync.Pool) {
// 	startTime := time.Now()

// 	// --- Request Handling ---
// 	reqHeadersBytes, _ := json.Marshal(r.Header) // Capture original headers
// 	clientIP := getClientIP(r)

// 	// Clone request body if needed for recording/replay matching later
// 	var reqBodyBytes []byte
// 	var reqBodyErr error
// 	if config.RecordingMode && r.Body != nil && r.ContentLength != 0 {
// 		buf := getBuffer() // Use pooled buffer
// 		defer putBuffer(buf)

// 		// Use TeeReader to simultaneously read into buffer and preserve original body
// 		teeReader := io.TeeReader(r.Body, buf)

// 		// Need to read the entire body into the buffer to capture it
// 		// This might be inefficient for very large requests. Consider alternatives if needed.
// 		_, errRead := io.ReadAll(teeReader)
// 		if errRead != nil {
// 			reqBodyErr = fmt.Errorf("reading request body for recording: %w", errRead)
// 			log.Printf("⚠️ Error cloning request body for %s %s: %v", r.Method, r.URL.String(), reqBodyErr)
// 			// Don't fail the request, just proceed without recording the body
// 		} else {
// 			reqBodyBytes = buf.Bytes()
// 			// Restore the request body so the proxy can read it
// 			r.Body = io.NopCloser(bytes.NewReader(reqBodyBytes))
// 		}

// 	} else if r.Body != nil {
// 		// Ensure body is closed even if not recording
// 		defer r.Body.Close()
// 	}

// 	// --- Replay Mode ---
// 	if config.ReplayMode {
// 		replayHTTPTraffic(w, r)
// 		return // Replay handled the response
// 	}

// 	// --- Recording or Passthrough Mode ---
// 	// Use a ResponseWriter wrapper only if recording
// 	var recorder *responseRecorder
// 	writer := w // Default to original writer

// 	if config.RecordingMode {
// 		responseBuf := responseBufPool.Get().(*bytes.Buffer)
// 		responseBuf.Reset()                    // Ensure buffer is empty
// 		defer responseBufPool.Put(responseBuf) // Return buffer to pool

// 		recorder = &responseRecorder{
// 			ResponseWriter: w,
// 			statusCode:     http.StatusOK, // Default
// 			body:           responseBuf,
// 			header:         http.Header{},
// 		}
// 		writer = recorder
// 	}

// 	// Serve the request using the proxy
// 	proxy.ServeHTTP(writer, r)

// 	// --- Recording (after response) ---
// 	if config.RecordingMode && recorder != nil {
// 		// Calculate duration
// 		duration := time.Since(startTime).Milliseconds()

// 		// Capture response details
// 		respHeadersBytes, _ := json.Marshal(recorder.Header())
// 		respBodyBytes := recorder.body.Bytes()

// 		// Save the record asynchronously
// 		go func(record TrafficRecord) {
// 			if err := saveTrafficRecord(record); err != nil {
// 				log.Printf("⚠️ Error saving recorded HTTP traffic: %v", err)
// 			}
// 		}(TrafficRecord{
// 			ID:              generateID(),
// 			Timestamp:       time.Now().UTC(),
// 			Protocol:        "HTTP",
// 			Method:          r.Method,
// 			URL:             r.URL.String(),
// 			RequestHeaders:  string(reqHeadersBytes),
// 			RequestBody:     reqBodyBytes, // Recorded request body
// 			ResponseStatus:  recorder.statusCode,
// 			ResponseHeaders: string(respHeadersBytes),
// 			ResponseBody:    respBodyBytes,
// 			Duration:        duration,
// 			ClientIP:        clientIP,
// 			SessionID:       r.Header.Get("X-Session-ID"), // Standardize header names
// 			TestID:          r.Header.Get("X-Test-ID"),
// 		})
// 	}
// }

// // responseRecorder wrapper captures status code, headers, and body
// type responseRecorder struct {
// 	http.ResponseWriter
// 	statusCode int
// 	header     http.Header
// 	body       *bytes.Buffer // Use a buffer for efficient writing
// }

// // Header captures headers
// func (r *responseRecorder) Header() http.Header {
// 	// Ensure headers are initialized from the underlying writer if not already set
// 	if len(r.header) == 0 {
// 		originalHeaders := r.ResponseWriter.Header()
// 		for k, v := range originalHeaders {
// 			r.header[k] = v
// 		}
// 	}
// 	return r.header
// }

// // WriteHeader captures status code and writes header to underlying writer
// func (r *responseRecorder) WriteHeader(statusCode int) {
// 	r.statusCode = statusCode
// 	// Write headers captured *before* writing the status code
// 	for k, v := range r.header {
// 		r.ResponseWriter.Header()[k] = v
// 	}
// 	r.ResponseWriter.WriteHeader(statusCode)
// }

// // Write captures body and writes to underlying writer
// func (r *responseRecorder) Write(b []byte) (int, error) {
// 	// Write to our buffer first
// 	n, err := r.body.Write(b)
// 	if err != nil {
// 		return n, err // Return error from buffer write if any
// 	}
// 	// Then write to the original ResponseWriter
// 	return r.ResponseWriter.Write(b)
// }

// // replayHTTPTraffic serves a response from the database
// func replayHTTPTraffic(w http.ResponseWriter, r *http.Request) {
// 	// Consider matching on headers or body hash for more accuracy
// 	query := `SELECT response_status, response_headers, response_body
//               FROM traffic_records
//               WHERE protocol = 'HTTP' AND method = ? AND url = ?
//               ORDER BY timestamp DESC LIMIT 1`

// 	row := db.QueryRow(query, r.Method, r.URL.String())

// 	var status int
// 	var headersStr string
// 	var respBody []byte

// 	err := row.Scan(&status, &headersStr, &respBody)
// 	if err != nil {
// 		if errors.Is(err, sql.ErrNoRows) {
// 			log.Printf("🕵️ No replay record found for HTTP %s %s", r.Method, r.URL.String())
// 			http.Error(w, "No matching replay record found", http.StatusNotFound)
// 		} else {
// 			log.Printf("🚨 DB error during HTTP replay lookup for %s %s: %v", r.Method, r.URL.String(), err)
// 			http.Error(w, "Database error during replay", http.StatusInternalServerError)
// 		}
// 		return
// 	}

// 	// Parse and set headers
// 	var headers http.Header
// 	if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
// 		log.Printf("⚠️ Error parsing stored headers for HTTP %s %s: %v", r.Method, r.URL.String(), err)
// 		// Proceed without headers
// 	} else {
// 		for name, values := range headers {
// 			// Header().Set overrides, Add appends. Use Add for multi-value headers.
// 			for _, value := range values {
// 				w.Header().Add(name, value)
// 			}
// 		}
// 	}

// 	// Set status code and write response body
// 	w.WriteHeader(status)
// 	if len(respBody) > 0 {
// 		_, err := w.Write(respBody)
// 		if err != nil {
// 			// Log error if writing response fails (e.g., client disconnected)
// 			log.Printf("⚠️ Error writing replayed HTTP response for %s %s: %v", r.Method, r.URL.String(), err)
// 		}
// 	}
// 	log.Printf("🔁 Replayed HTTP %d for %s %s", status, r.Method, r.URL.String())
// }

// // saveTrafficRecord saves a traffic record to SQLite
// func saveTrafficRecord(record TrafficRecord) error {
// 	// Ensure context with timeout for DB operations? Maybe overkill for insert.
// 	_, err := insertStmt.Exec(
// 		record.ID,
// 		record.Timestamp,
// 		record.Protocol,
// 		record.Method,
// 		record.URL,
// 		record.RequestHeaders,
// 		record.RequestBody,
// 		record.ResponseStatus,
// 		record.ResponseHeaders,
// 		record.ResponseBody,
// 		record.Duration,
// 		record.ClientIP,
// 		record.TestID,
// 		record.SessionID,
// 	)
// 	if err != nil {
// 		// Log the specific record ID if saving failed
// 		return fmt.Errorf("saving record %s: %w", record.ID, err)
// 	}
// 	// Log success implicitly via the calling function if needed
// 	return nil
// }

// // getClientIP extracts the client IP from the request
// func getClientIP(r *http.Request) string {
// 	// Check common headers first (useful behind load balancers)
// 	ip := r.Header.Get("X-Forwarded-For")
// 	if ip != "" {
// 		// X-Forwarded-For can be a list, take the first one
// 		parts := strings.Split(ip, ",")
// 		return strings.TrimSpace(parts[0])
// 	}
// 	ip = r.Header.Get("X-Real-IP")
// 	if ip != "" {
// 		return strings.TrimSpace(ip)
// 	}
// 	// Fallback to remote address
// 	ip, _, err := net.SplitHostPort(r.RemoteAddr)
// 	if err != nil {
// 		return r.RemoteAddr // Return full address if split fails
// 	}
// 	return ip
// }

// var idCounter uint64 = 0

// func generateID() string {
// 	// Combine timestamp with atomic counter to avoid collisions
// 	id := atomic.AddUint64(&idCounter, 1)

// 	// Add some randomness
// 	buf := make([]byte, 4)
// 	rand.Read(buf)

// 	return fmt.Sprintf("%d-%x", id, hex.EncodeToString(buf))
// }

// // setupDatabase creates schema and prepares insert statement
// func setupDatabase() error {
// 	// Use TEXT for JSON fields, BLOB for binary bodies
// 	// Added indexes likely to be used in filtering/searching
// 	query := `
//     CREATE TABLE IF NOT EXISTS traffic_records (
//         id TEXT PRIMARY KEY,
//         timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
//         protocol TEXT NOT NULL,
//         method TEXT NOT NULL,
//         url TEXT,
//         service TEXT,
//         request_headers TEXT,
//         request_body BLOB,
//         response_status INTEGER NOT NULL,
//         response_headers TEXT,
//         response_body BLOB,
//         duration_ms INTEGER,
//         client_ip TEXT,
//         test_id TEXT,
//         session_id TEXT
//     );

//     -- Index for HTTP replay/lookup
//     CREATE INDEX IF NOT EXISTS idx_http_lookup ON traffic_records(protocol, method, url) WHERE protocol = 'HTTP';
//     -- Index for searching by time
//     CREATE INDEX IF NOT EXISTS idx_timestamp ON traffic_records(timestamp);
//     -- Index for searching by session or test ID
//     CREATE INDEX IF NOT EXISTS idx_session_id ON traffic_records(session_id) WHERE session_id IS NOT NULL;
//     CREATE INDEX IF NOT EXISTS idx_test_id ON traffic_records(test_id) WHERE test_id IS NOT NULL;
//     `

// 	_, err := db.Exec(query)
// 	if err != nil {
// 		return fmt.Errorf("creating schema: %w", err)
// 	}

// 	// Prepare statement for inserts
// 	insertSQL := `INSERT INTO traffic_records (
//         id, timestamp, protocol, method, url, service,
//         request_headers, request_body, response_status,
//         response_headers, response_body, duration_ms,
//         client_ip, test_id, session_id
//     ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// 	insertStmt, err = db.Prepare(insertSQL)
// 	if err != nil {
// 		return fmt.Errorf("preparing insert statement: %w", err)
// 	}

// 	log.Println("✅ Database schema verified and statement prepared")
// 	return nil
// }
