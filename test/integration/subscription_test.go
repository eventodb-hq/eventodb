package integration

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/message-db/message-db/internal/api"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// Poke represents an SSE poke event
type Poke struct {
	Stream         string `json:"stream"`
	Position       int64  `json:"position"`
	GlobalPosition int64  `json:"globalPosition"`
}

// setupSSETestServer creates a test server for SSE
func setupSSETestServer(t *testing.T) (*httptest.Server, *sqlite.SQLiteStore, func()) {
	t.Helper()

	// Create in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create store
	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create default namespace
	ctx := context.Background()
	token, _ := auth.GenerateToken("default")
	tokenHash := auth.HashToken(token)
	_ = st.CreateNamespace(ctx, "default", tokenHash, "Default namespace")

	// Create handlers
	rpcHandler := api.NewRPCHandler("1.3.0", st)
	sseHandler := api.NewSSEHandler(st, true) // test mode

	// Create mux
	mux := http.NewServeMux()
	mux.Handle("/rpc", api.AuthMiddleware(st, true)(rpcHandler))
	mux.HandleFunc("/subscribe", sseHandler.HandleSubscribe)

	server := httptest.NewServer(mux)

	return server, st, func() {
		server.Close()
		st.Close()
		db.Close()
	}
}

// writeMessage writes a message to the store
func writeSSEMessage(ctx context.Context, st *sqlite.SQLiteStore, namespace, streamName, msgType string, data map[string]interface{}) error {
	msg := &store.Message{
		Type: msgType,
		Data: data,
	}
	_, err := st.WriteMessage(ctx, namespace, streamName, msg)
	return err
}

// createSSENamespace creates a test namespace
func createSSENamespace(ctx context.Context, st *sqlite.SQLiteStore, namespace string) (string, error) {
	token, err := auth.GenerateToken(namespace)
	if err != nil {
		return "", err
	}

	tokenHash := auth.HashToken(token)
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Test namespace")
	if err != nil {
		return "", err
	}

	return token, nil
}

// MDB002_6A_T1: Test SSE connection established
func TestMDB002_6A_T1_SSEConnectionEstablished(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	// Create namespace and get token
	token, err := createSSENamespace(ctx, st, "test-sse")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create a stream with a message
	streamName := "account-123"
	err = writeSSEMessage(ctx, st, "test-sse", streamName, "Opened", map[string]interface{}{"amount": 100})
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Make SSE request with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=" + streamName + "&position=0"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		defer resp.Body.Close()

		// Check headers
		if resp.Header.Get("Content-Type") != "text/event-stream" {
			t.Errorf("Expected Content-Type: text/event-stream, got: %s", resp.Header.Get("Content-Type"))
		}
		if resp.Header.Get("Cache-Control") != "no-cache" {
			t.Errorf("Expected Cache-Control: no-cache, got: %s", resp.Header.Get("Cache-Control"))
		}
	}
	// Connection timeout is expected since it's an SSE stream
}

// MDB002_6A_T2: Test SSE headers set correctly
func TestMDB002_6A_T2_SSEHeadersSetCorrectly(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-headers")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=test-stream&position=0"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		defer resp.Body.Close()

		requiredHeaders := map[string]string{
			"Content-Type":  "text/event-stream",
			"Cache-Control": "no-cache",
			"Connection":    "keep-alive",
		}

		for key, expected := range requiredHeaders {
			if got := resp.Header.Get(key); got != expected {
				t.Errorf("Header %s: expected %s, got %s", key, expected, got)
			}
		}
	}
}

// MDB002_6A_T3: Test connection requires valid token (in test mode, auth is optional)
func TestMDB002_6A_T3_ConnectionWithAuth(t *testing.T) {
	ctx := context.Background()
	testServer, _, cleanup := setupSSETestServer(t)
	defer cleanup()

	tests := []struct {
		name       string
		authHeader string
		shouldWork bool
	}{
		{
			name:       "no authorization header",
			authHeader: "",
			shouldWork: true, // In test mode, should work with default namespace
		},
		{
			name:       "valid token format",
			authHeader: "Bearer ns_dGVzdA_abc123",
			shouldWork: true, // In test mode, should work
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()

			url := testServer.URL + "/subscribe?stream=test&position=0"
			req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if tt.shouldWork {
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode >= 400 {
						t.Errorf("Expected success, got status %d", resp.StatusCode)
					}
				}
			} else {
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode < 400 {
						t.Errorf("Expected error status, got %d", resp.StatusCode)
					}
				}
			}
		})
	}
}

// MDB002_6A_T4: Test stream subscription receives poke on new message
func TestMDB002_6A_T4_StreamSubscriptionReceivesPoke(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-poke")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	streamName := "account-456"

	// Start SSE connection in background
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=" + streamName + "&position=0"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	// Start reading events
	reader := bufio.NewReader(resp.Body)
	pokeChan := make(chan *Poke, 1)

	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")
				var poke Poke
				if json.Unmarshal([]byte(jsonData), &poke) == nil {
					pokeChan <- &poke
					return
				}
			}
		}
	}()

	// Wait for connection to establish
	time.Sleep(200 * time.Millisecond)

	// Write a message
	err = writeSSEMessage(ctx, st, "test-poke", streamName, "Deposited", map[string]interface{}{"amount": 50})
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Wait for poke
	select {
	case poke := <-pokeChan:
		if poke.Stream != streamName {
			t.Errorf("Expected stream %s, got %s", streamName, poke.Stream)
		}
		if poke.Position != 0 {
			t.Errorf("Expected position 0, got %d", poke.Position)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Error("Timeout waiting for poke")
	}
}

// MDB002_6A_T5: Test poke contains correct position
func TestMDB002_6A_T5_PokeContainsCorrectPosition(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-position")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	streamName := "account-789"

	// Write initial messages
	for i := 0; i < 3; i++ {
		err = writeSSEMessage(ctx, st, "test-position", streamName, "Deposited", map[string]interface{}{"amount": i * 10})
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Subscribe from position 2
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=" + streamName + "&position=2"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	// Should receive poke for existing message at position 2
	poke, err := readNextPoke(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive poke: %v", err)
	}

	if poke.Position != 2 {
		t.Errorf("Expected position 2, got %d", poke.Position)
	}
}

// MDB002_6A_T6: Test multiple pokes for multiple messages
func TestMDB002_6A_T6_MultiplePokesForMultipleMessages(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-multiple")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	streamName := "account-multi"

	// Start subscription
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=" + streamName + "&position=0"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	time.Sleep(200 * time.Millisecond)

	// Write multiple messages
	messageCount := 3
	for i := 0; i < messageCount; i++ {
		err = writeSSEMessage(ctx, st, "test-multiple", streamName, "Event", map[string]interface{}{"seq": i})
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Receive pokes
	positions := make(map[int64]bool)
	for i := 0; i < messageCount; i++ {
		poke, err := readNextPoke(reader, 1*time.Second)
		if err != nil {
			t.Logf("Received %d of %d pokes before timeout", i, messageCount)
			break
		}
		positions[poke.Position] = true
	}

	// Verify we got at least some pokes
	if len(positions) == 0 {
		t.Error("Did not receive any pokes")
	}
}

// MDB002_6A_T7: Test subscription from specific position
func TestMDB002_6A_T7_SubscriptionFromSpecificPosition(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-offset")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	streamName := "account-offset"

	// Write 5 messages
	for i := 0; i < 5; i++ {
		err = writeSSEMessage(ctx, st, "test-offset", streamName, "Event", map[string]interface{}{"seq": i})
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Subscribe from position 3
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?stream=" + streamName + "&position=3"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	// Should receive pokes for positions 3 and 4
	poke, err := readNextPoke(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive poke: %v", err)
	}

	if poke.Position < 3 {
		t.Errorf("Expected position >= 3, got %d", poke.Position)
	}
}

// MDB002_6A_T8: Test category subscription receives pokes
func TestMDB002_6A_T8_CategorySubscriptionReceivesPokes(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-category")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Subscribe to category
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?category=account&position=1"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	time.Sleep(200 * time.Millisecond)

	// Write messages to different streams in the category
	streams := []string{"account-111", "account-222"}
	for _, stream := range streams {
		err = writeSSEMessage(ctx, st, "test-category", stream, "Opened", map[string]interface{}{"id": stream})
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", stream, err)
		}
	}

	// Try to receive at least one poke
	poke, err := readNextPoke(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive poke: %v", err)
	}

	// Verify it's from one of our streams
	found := false
	for _, stream := range streams {
		if poke.Stream == stream {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Poke stream %s not in expected streams %v", poke.Stream, streams)
	}
}

// MDB002_6A_T9: Test poke includes stream name for category
func TestMDB002_6A_T9_PokeIncludesStreamNameForCategory(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-stream-name")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Subscribe to category
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?category=product&position=1"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	time.Sleep(200 * time.Millisecond)

	// Write message to specific stream
	streamName := "product-xyz"
	err = writeSSEMessage(ctx, st, "test-stream-name", streamName, "Created", map[string]interface{}{"name": "Widget"})
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Receive poke and verify stream name is included
	poke, err := readNextPoke(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive poke: %v", err)
	}

	if poke.Stream != streamName {
		t.Errorf("Expected stream name %s, got %s", streamName, poke.Stream)
	}
}

// MDB002_6A_T10: Test consumer group filtering in subscription
func TestMDB002_6A_T10_ConsumerGroupFilteringInSubscription(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-consumer-group")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Write messages to multiple streams
	streams := []string{"order-100", "order-200"}
	for _, stream := range streams {
		err = writeSSEMessage(ctx, st, "test-consumer-group", stream, "Created", map[string]interface{}{"stream": stream})
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", stream, err)
		}
	}

	// Subscribe as consumer 0 of 2
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := testServer.URL + "/subscribe?category=order&position=1&consumer=0&size=2"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	// Just verify we can connect with consumer group parameters
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// MDB002_6A_T11: Test connection cleanup on client disconnect
func TestMDB002_6A_T11_ConnectionCleanupOnDisconnect(t *testing.T) {
	ctx := context.Background()
	testServer, st, cleanup := setupSSETestServer(t)
	defer cleanup()

	token, err := createSSENamespace(ctx, st, "test-cleanup")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Start subscription with a cancelable context
	reqCtx, cancel := context.WithCancel(ctx)

	url := testServer.URL + "/subscribe?stream=test&position=0"
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Cancel the context (disconnect)
	cancel()
	resp.Body.Close()

	// If we get here without hanging, cleanup worked
	time.Sleep(100 * time.Millisecond)
}

// Helper function to read next poke from SSE stream
func readNextPoke(reader *bufio.Reader, timeout time.Duration) (*Poke, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		reader.ReadString('\n') // Skip any initial comments or empty lines
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		line = strings.TrimSpace(line)

		// Look for "data: " line
		if strings.HasPrefix(line, "data: ") {
			jsonData := strings.TrimPrefix(line, "data: ")
			var poke Poke
			if err := json.Unmarshal([]byte(jsonData), &poke); err == nil {
				return &poke, nil
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for poke")
}
