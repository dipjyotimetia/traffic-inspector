package db

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestInitialize(t *testing.T) {
	// Create a temporary file for the database
	tempFile, err := os.CreateTemp("", "traffic_inspector_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Test initialization
	database, stmt, err := Initialize(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	defer stmt.Close()

	// Check that the database connection works
	err = database.Ping()
	if err != nil {
		t.Errorf("Failed to ping database after initialization: %v", err)
	}
}

func TestTrafficRecordStorage(t *testing.T) {
	// Create a temporary file for the database
	tempFile, err := os.CreateTemp("", "traffic_inspector_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Initialize database
	database, stmt, err := Initialize(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	defer stmt.Close()

	// Test data
	testRecords := []TrafficRecord{
		{
			ID:              "test-record-1",
			Timestamp:       time.Now().UTC(),
			Protocol:        "HTTP",
			Method:          "GET",
			URL:             "https://example.com/api/users",
			RequestHeaders:  `{"Content-Type": ["application/json"]}`,
			RequestBody:     []byte(`{"query": "users"}`),
			ResponseStatus:  200,
			ResponseHeaders: `{"Content-Type": ["application/json"]}`,
			ResponseBody:    []byte(`["user1", "user2"]`),
			Duration:        150,
			ClientIP:        "192.168.1.1",
			TestID:          "test-123",
			SessionID:       "session-abc",
		},
		{
			ID:              "test-record-2",
			Timestamp:       time.Now().UTC(),
			Protocol:        "HTTP",
			Method:          "POST",
			URL:             "https://example.com/api/products",
			RequestHeaders:  `{"Content-Type": ["application/json"]}`,
			RequestBody:     []byte(`{"name": "Product 1"}`),
			ResponseStatus:  201,
			ResponseHeaders: `{"Content-Type": ["application/json"]}`,
			ResponseBody:    []byte(`{"id": "p1", "name": "Product 1"}`),
			Duration:        120,
			ClientIP:        "192.168.1.2",
			TestID:          "test-456",
			SessionID:       "session-def",
		},
	}

	// Insert test records
	for _, record := range testRecords {
		_, err := stmt.Exec(
			record.ID,
			record.Timestamp,
			record.Protocol,
			record.Method,
			record.URL,
			"", // service field
			record.RequestHeaders,
			record.RequestBody,
			record.ResponseStatus,
			record.ResponseHeaders,
			record.ResponseBody,
			record.Duration,
			record.ClientIP,
			record.TestID,
			record.SessionID,
		)
		if err != nil {
			t.Fatalf("Failed to insert test record: %v", err)
		}
	}

	// Test queries
	t.Run("QueryByID", func(t *testing.T) {
		var id, protocol, method, url string
		var status int

		err := database.QueryRow("SELECT id, protocol, method, url, response_status FROM traffic_records WHERE id = ?", "test-record-1").
			Scan(&id, &protocol, &method, &url, &status)
		if err != nil {
			t.Fatalf("Failed to query record: %v", err)
		}

		if id != "test-record-1" || protocol != "HTTP" || method != "GET" ||
			url != "https://example.com/api/users" || status != 200 {
			t.Errorf("Retrieved record doesn't match: got id=%s, protocol=%s, method=%s, url=%s, status=%d",
				id, protocol, method, url, status)
		}
	})

	t.Run("QueryByMethodAndURL", func(t *testing.T) {
		var id string
		var status int

		err := database.QueryRow(
			"SELECT id, response_status FROM traffic_records WHERE method = ? AND url = ?",
			"POST", "https://example.com/api/products",
		).Scan(&id, &status)
		if err != nil {
			t.Fatalf("Failed to query record by method and URL: %v", err)
		}

		if id != "test-record-2" || status != 201 {
			t.Errorf("Retrieved record doesn't match: got id=%s, status=%d", id, status)
		}
	})

	t.Run("QueryBySession", func(t *testing.T) {
		var count int

		err := database.QueryRow(
			"SELECT COUNT(*) FROM traffic_records WHERE session_id = ?",
			"session-abc",
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count records by session: %v", err)
		}

		if count != 1 {
			t.Errorf("Wrong count for session-abc: got %d, want 1", count)
		}
	})
}

func TestDatabaseIndexes(t *testing.T) {
	// Create a temporary file for the database
	tempFile, err := os.CreateTemp("", "traffic_inspector_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Initialize database
	database, stmt, err := Initialize(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	defer stmt.Close()

	// Check that indexes exist
	rows, err := database.Query("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'traffic_records'")
	if err != nil {
		t.Fatalf("Failed to query indexes: %v", err)
	}
	defer rows.Close()

	indexes := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan index name: %v", err)
		}
		indexes[name] = true
	}

	// Verify expected indexes
	expectedIndexes := []string{
		"idx_http_lookup",
		"idx_timestamp",
		"idx_session_id",
		"idx_test_id",
	}

	for _, idx := range expectedIndexes {
		if !indexes[idx] {
			t.Errorf("Expected index %s not found", idx)
		}
	}
}

func TestDatabaseFailure(t *testing.T) {
	// Test with an invalid database path
	_, _, err := Initialize("/nonexistent/directory/db.sqlite")
	if err == nil {
		t.Error("Expected error with invalid database path, but got nil")
	}

	// Test with a read-only directory
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only test when running as root")
	}

	tempDir, err := os.MkdirTemp("", "readonly_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Make directory read-only
	err = os.Chmod(tempDir, 0o500)
	if err != nil {
		t.Fatalf("Failed to set directory permissions: %v", err)
	}

	dbPath := tempDir + "/test.db"
	_, _, err = Initialize(dbPath)
	if err == nil {
		t.Error("Expected error with read-only directory, but got nil")
	}
}

func TestDatabaseConcurrency(t *testing.T) {
	// Create a temporary file for the database
	tempFile, err := os.CreateTemp("", "traffic_inspector_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Initialize database
	database, stmt, err := Initialize(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	defer stmt.Close()

	// Number of concurrent operations
	const numConcurrent = 10
	errCh := make(chan error, numConcurrent)
	doneCh := make(chan bool, numConcurrent)

	// Use a mutex to protect ID generation
	var idMutex sync.Mutex
	idCounter := 0

	// Insert records concurrently
	for i := range numConcurrent {
		go func(idx int) {
			// Generate a truly unique ID
			idMutex.Lock()
			idCounter++
			uniqueID := fmt.Sprintf("concurrent-%d-%d", idx, idCounter)
			idMutex.Unlock()

			record := TrafficRecord{
				ID:              uniqueID,
				Timestamp:       time.Now().UTC(),
				Protocol:        "HTTP",
				Method:          "GET",
				URL:             fmt.Sprintf("https://example.com/api/concurrent/%d", idx),
				RequestHeaders:  `{"Content-Type": ["application/json"]}`,
				RequestBody:     []byte(fmt.Sprintf(`{"worker": %d}`, idx)),
				ResponseStatus:  200,
				ResponseHeaders: `{"Content-Type": ["application/json"]}`,
				ResponseBody:    []byte(`{"result": "ok"}`),
				Duration:        int64(100 + idx),
				ClientIP:        fmt.Sprintf("192.168.1.%d", idx),
				TestID:          fmt.Sprintf("test-%d", idx),
				SessionID:       fmt.Sprintf("session-%d", idx),
			}

			_, err := stmt.Exec(
				record.ID,
				record.Timestamp,
				record.Protocol,
				record.Method,
				record.URL,
				"", // service field
				record.RequestHeaders,
				record.RequestBody,
				record.ResponseStatus,
				record.ResponseHeaders,
				record.ResponseBody,
				record.Duration,
				record.ClientIP,
				record.TestID,
				record.SessionID,
			)
			if err != nil {
				errCh <- err
				return
			}

			doneCh <- true
		}(i)
	}

	// Collect results
	successCount := 0
	errorCount := 0

	for range numConcurrent {
		select {
		case <-doneCh:
			successCount++
		case err := <-errCh:
			errorCount++
			t.Logf("Concurrent insert error: %v", err)
		}
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors during concurrent inserts", errorCount)
	}

	// Verify total count
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM traffic_records").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}

	if count != numConcurrent {
		t.Errorf("Expected %d records, got %d", numConcurrent, count)
	}
}
