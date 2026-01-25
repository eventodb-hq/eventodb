package integration

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
)

// TestMDB004_4A_T1_ImportSendsFileAsChunkedBody tests that import sends file content to server
func TestMDB004_4A_T1_ImportSendsFileAsChunkedBody(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create temp file with NDJSON content
	tmpFile, err := os.CreateTemp("", "import-test-*.ndjson")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test records
	records := []string{
		fmt.Sprintf(`{"id":"%s","stream":"test-1","type":"Created","pos":0,"gpos":1,"data":{"value":1},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"test-2","type":"Created","pos":0,"gpos":2,"data":{"value":2},"meta":null,"time":"2025-01-15T10:01:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"test-3","type":"Created","pos":0,"gpos":3,"data":{"value":3},"meta":null,"time":"2025-01-15T10:02:00Z"}`, uuid.New().String()),
	}
	for _, rec := range records {
		tmpFile.WriteString(rec + "\n")
	}
	tmpFile.Close()

	// Open file and send to server (simulating CLI behavior)
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to open temp file: %v", err)
	}
	defer file.Close()

	req, err := http.NewRequest("POST", server.URL()+"/import", file)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify done event
	found := parseSSEForDone(t, resp.Body, 3)
	if !found {
		t.Error("Expected done event with 3 imported messages")
	}

	// Verify messages are in store
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		streamName := fmt.Sprintf("test-%d", i)
		msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, streamName, nil)
		if err != nil {
			t.Errorf("Failed to get stream %s: %v", streamName, err)
		}
		if len(msgs) != 1 {
			t.Errorf("Expected 1 message in stream %s, got %d", streamName, len(msgs))
		}
	}
}

// TestMDB004_4A_T2_ImportWithGzipDecompressesInput tests gzip decompression
func TestMDB004_4A_T2_ImportWithGzipDecompressesInput(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create gzip compressed NDJSON content
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)

	records := []string{
		fmt.Sprintf(`{"id":"%s","stream":"gzip-1","type":"Created","pos":0,"gpos":100,"data":{"test":"gzip"},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"gzip-2","type":"Created","pos":0,"gpos":200,"data":{"test":"gzip"},"meta":null,"time":"2025-01-15T10:01:00Z"}`, uuid.New().String()),
	}
	for _, rec := range records {
		gzWriter.Write([]byte(rec + "\n"))
	}
	gzWriter.Close()

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "import-test-*.ndjson.gz")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(compressed.Bytes())
	tmpFile.Close()

	// Open and decompress (simulating CLI --gzip flag)
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to open temp file: %v", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Read decompressed content
	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	// Send decompressed content to server
	req, err := http.NewRequest("POST", server.URL()+"/import", bytes.NewReader(decompressed))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify done event
	found := parseSSEForDone(t, resp.Body, 2)
	if !found {
		t.Error("Expected done event with 2 imported messages")
	}

	// Verify messages are in store
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, "gzip-1", nil)
	if err != nil {
		t.Fatalf("Failed to get stream: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].GlobalPosition != 100 {
		t.Errorf("Expected gpos 100, got %d", msgs[0].GlobalPosition)
	}
}

// TestMDB004_4A_T3_ImportDisplaysProgressUpdates tests progress event parsing
func TestMDB004_4A_T3_ImportDisplaysProgressUpdates(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create 1500 records to trigger at least one progress event
	var buf bytes.Buffer
	for i := 0; i < 1500; i++ {
		rec := fmt.Sprintf(
			`{"id":"%s","stream":"progress-%d","type":"Created","pos":0,"gpos":%d,"data":{"idx":%d},"meta":null,"time":"2025-01-15T10:00:00Z"}`,
			uuid.New().String(), i, i+1, i,
		)
		buf.WriteString(rec + "\n")
	}

	req, err := http.NewRequest("POST", server.URL()+"/import", &buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Parse SSE events and verify we get progress updates
	var progressEvents []struct {
		Imported int64 `json:"imported"`
		GPos     int64 `json:"gpos"`
	}
	var doneEvent struct {
		Done     bool   `json:"done"`
		Imported int64  `json:"imported"`
		Elapsed  string `json:"elapsed"`
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if _, hasDone := event["done"]; hasDone {
			json.Unmarshal([]byte(data), &doneEvent)
		} else if _, hasImported := event["imported"]; hasImported {
			var p struct {
				Imported int64 `json:"imported"`
				GPos     int64 `json:"gpos"`
			}
			json.Unmarshal([]byte(data), &p)
			progressEvents = append(progressEvents, p)
		}
	}

	// Should have at least 1 progress event (at 1000 records)
	if len(progressEvents) < 1 {
		t.Errorf("Expected at least 1 progress event, got %d", len(progressEvents))
	}

	// First progress should show 1000 imported
	if len(progressEvents) > 0 && progressEvents[0].Imported != 1000 {
		t.Errorf("Expected first progress at 1000 imported, got %d", progressEvents[0].Imported)
	}

	// Should have done event with correct total
	if !doneEvent.Done {
		t.Error("Expected done event")
	}
	if doneEvent.Imported != 1500 {
		t.Errorf("Expected 1500 imported in done event, got %d", doneEvent.Imported)
	}
	if doneEvent.Elapsed == "" {
		t.Error("Expected elapsed time in done event")
	}
}

// TestMDB004_4A_T4_ImportHandlesServerErrors tests error handling
func TestMDB004_4A_T4_ImportHandlesServerErrors(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// First, import a record with a specific gpos
	rec1 := fmt.Sprintf(`{"id":"%s","stream":"error-test","type":"First","pos":0,"gpos":999,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String())

	req1, _ := http.NewRequest("POST", server.URL()+"/import", strings.NewReader(rec1))
	req1.Header.Set("Authorization", "Bearer "+server.Token)

	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("First import failed: %v", err)
	}
	resp1.Body.Close()

	// Now try to import with duplicate gpos - should fail
	rec2 := fmt.Sprintf(`{"id":"%s","stream":"error-test","type":"Duplicate","pos":1,"gpos":999,"data":{},"meta":null,"time":"2025-01-15T10:00:01Z"}`, uuid.New().String())

	req2, _ := http.NewRequest("POST", server.URL()+"/import", strings.NewReader(rec2))
	req2.Header.Set("Authorization", "Bearer "+server.Token)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("Second import request failed: %v", err)
	}
	defer resp2.Body.Close()

	// Parse SSE for error event
	errCode, _ := parseSSEForError(t, resp2.Body)
	if errCode != "POSITION_EXISTS" {
		t.Errorf("Expected POSITION_EXISTS error, got %q", errCode)
	}
}

// TestMDB004_4A_T5_ImportFromStdinWorks tests stdin-like input
func TestMDB004_4A_T5_ImportFromStdinWorks(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Simulate stdin by using a bytes.Buffer (which is what stdin would provide)
	var stdin bytes.Buffer
	records := []string{
		fmt.Sprintf(`{"id":"%s","stream":"stdin-1","type":"Created","pos":0,"gpos":50,"data":{"source":"stdin"},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"stdin-2","type":"Created","pos":0,"gpos":51,"data":{"source":"stdin"},"meta":null,"time":"2025-01-15T10:01:00Z"}`, uuid.New().String()),
	}
	for _, rec := range records {
		stdin.WriteString(rec + "\n")
	}

	req, err := http.NewRequest("POST", server.URL()+"/import", &stdin)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify done event
	found := parseSSEForDone(t, resp.Body, 2)
	if !found {
		t.Error("Expected done event with 2 imported messages")
	}

	// Verify messages are in store with correct global positions
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, "stdin-1", nil)
	if err != nil {
		t.Fatalf("Failed to get stream: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].GlobalPosition != 50 {
		t.Errorf("Expected gpos 50, got %d", msgs[0].GlobalPosition)
	}
}

// TestMDB004_4A_T6_ImportFromFileWorks tests file-based input
func TestMDB004_4A_T6_ImportFromFileWorks(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "import-file-test-*.ndjson")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write records to file
	records := []string{
		fmt.Sprintf(`{"id":"%s","stream":"file-a","type":"Alpha","pos":0,"gpos":101,"data":{"file":true},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"file-b","type":"Beta","pos":0,"gpos":102,"data":{"file":true},"meta":null,"time":"2025-01-15T10:01:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"file-c","type":"Gamma","pos":0,"gpos":103,"data":{"file":true},"meta":null,"time":"2025-01-15T10:02:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"file-d","type":"Delta","pos":0,"gpos":104,"data":{"file":true},"meta":null,"time":"2025-01-15T10:03:00Z"}`, uuid.New().String()),
	}
	for _, rec := range records {
		tmpFile.WriteString(rec + "\n")
	}
	tmpFile.Close()

	// Read file and send (simulating CLI behavior)
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	req, err := http.NewRequest("POST", server.URL()+"/import", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify done event
	found := parseSSEForDone(t, resp.Body, 4)
	if !found {
		t.Error("Expected done event with 4 imported messages")
	}

	// Verify all messages exist
	ctx := context.Background()
	streams := []string{"file-a", "file-b", "file-c", "file-d"}
	for _, stream := range streams {
		msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, stream, nil)
		if err != nil {
			t.Errorf("Failed to get stream %s: %v", stream, err)
			continue
		}
		if len(msgs) != 1 {
			t.Errorf("Expected 1 message in %s, got %d", stream, len(msgs))
		}
	}
}

// TestMDB004_4A_ImportPreservesAllFields tests that all message fields are preserved
func TestMDB004_4A_ImportPreservesAllFields(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	msgID := uuid.New().String()
	record := fmt.Sprintf(`{"id":"%s","stream":"complete-msg","type":"FullMessage","pos":5,"gpos":999,"data":{"amount":123.45,"items":["a","b"]},"meta":{"user":"test","action":"create"},"time":"2025-07-15T14:30:00Z"}`, msgID)

	req, _ := http.NewRequest("POST", server.URL()+"/import", strings.NewReader(record))
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	// Retrieve and verify all fields
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, "complete-msg", nil)
	if err != nil {
		t.Fatalf("Failed to get stream: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	msg := msgs[0]

	// Verify ID
	if msg.ID != msgID {
		t.Errorf("Expected ID %s, got %s", msgID, msg.ID)
	}

	// Verify stream name
	if msg.StreamName != "complete-msg" {
		t.Errorf("Expected stream 'complete-msg', got %s", msg.StreamName)
	}

	// Verify type
	if msg.Type != "FullMessage" {
		t.Errorf("Expected type 'FullMessage', got %s", msg.Type)
	}

	// Verify position (stream position)
	if msg.Position != 5 {
		t.Errorf("Expected position 5, got %d", msg.Position)
	}

	// Verify global position
	if msg.GlobalPosition != 999 {
		t.Errorf("Expected global position 999, got %d", msg.GlobalPosition)
	}

	// Verify data
	if msg.Data == nil {
		t.Error("Expected data to be non-nil")
	} else {
		if msg.Data["amount"] != 123.45 {
			t.Errorf("Expected amount 123.45, got %v", msg.Data["amount"])
		}
	}

	// Verify metadata
	if msg.Metadata == nil {
		t.Error("Expected metadata to be non-nil")
	} else {
		if msg.Metadata["user"] != "test" {
			t.Errorf("Expected user 'test', got %v", msg.Metadata["user"])
		}
	}

	// Verify time
	expectedTime, _ := time.Parse(time.RFC3339, "2025-07-15T14:30:00Z")
	if !msg.Time.Equal(expectedTime) {
		t.Errorf("Expected time %v, got %v", expectedTime, msg.Time)
	}
}

// TestMDB004_4A_ImportWithInvalidGzipFails tests invalid gzip handling
func TestMDB004_4A_ImportWithInvalidGzipFails(t *testing.T) {
	// This tests the CLI behavior - if --gzip is specified but file isn't gzip, it should fail
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create a non-gzip file
	tmpFile, err := os.CreateTemp("", "not-gzip-*.ndjson.gz")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write plain text (not gzip)
	tmpFile.WriteString(`{"id":"test","stream":"test","type":"A","pos":0,"gpos":1,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}`)
	tmpFile.Close()

	// Try to decompress (this should fail)
	file, _ := os.Open(tmpFile.Name())
	defer file.Close()

	_, err = gzip.NewReader(file)
	if err == nil {
		t.Error("Expected gzip reader to fail on non-gzip file")
	}
}

// TestMDB004_4A_ImportEmptyFile tests importing empty file
func TestMDB004_4A_ImportEmptyFile(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create empty file
	tmpFile, err := os.CreateTemp("", "empty-*.ndjson")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Read file (empty content)
	content, _ := os.ReadFile(tmpFile.Name())

	req, _ := http.NewRequest("POST", server.URL()+"/import", bytes.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for empty file, got %d", resp.StatusCode)
	}

	// Should return done with 0 imported
	found := parseSSEForDone(t, resp.Body, 0)
	if !found {
		t.Error("Expected done event with 0 imported")
	}
}

// TestMDB004_4A_ImportLargeFile tests importing larger files efficiently
func TestMDB004_4A_ImportLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create 5000 records
	var buf bytes.Buffer
	for i := 0; i < 5000; i++ {
		rec := fmt.Sprintf(
			`{"id":"%s","stream":"large-%d","type":"Record","pos":0,"gpos":%d,"data":{"index":%d,"padding":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"meta":null,"time":"2025-01-15T10:00:00Z"}`,
			uuid.New().String(), i, i+1, i,
		)
		buf.WriteString(rec + "\n")
	}

	req, err := http.NewRequest("POST", server.URL()+"/import", &buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify done event
	found := parseSSEForDone(t, resp.Body, 5000)
	elapsed := time.Since(start)

	if !found {
		t.Error("Expected done event with 5000 imported messages")
	}

	// Should complete in reasonable time (less than 30 seconds)
	if elapsed > 30*time.Second {
		t.Errorf("Import took too long: %v", elapsed)
	}

	// Verify count in database (spot check)
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, "large-0", nil)
	if err != nil {
		t.Fatalf("Failed to get stream: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message in large-0, got %d", len(msgs))
	}
}

// TestMDB004_4A_ImportInvalidJSONLineNumber tests that error reports correct line number
func TestMDB004_4A_ImportInvalidJSONLineNumber(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create content with invalid JSON on line 3
	body := fmt.Sprintf(`{"id":"%s","stream":"a","type":"A","pos":0,"gpos":1,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}
{"id":"%s","stream":"b","type":"B","pos":0,"gpos":2,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}
{this is not valid json}
{"id":"%s","stream":"d","type":"D","pos":0,"gpos":4,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}`,
		uuid.New().String(), uuid.New().String(), uuid.New().String())

	req, _ := http.NewRequest("POST", server.URL()+"/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should get error with line number 3
	errCode, errLine := parseSSEForError(t, resp.Body)
	if errCode != "INVALID_JSON" {
		t.Errorf("Expected INVALID_JSON error, got %q", errCode)
	}
	if errLine != 3 {
		t.Errorf("Expected error at line 3, got %d", errLine)
	}
}

// TestMDB004_4A_ImportPreservesStreamPosition tests that stream position is preserved
func TestMDB004_4A_ImportPreservesStreamPosition(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	// Import messages with specific stream positions
	records := []string{
		fmt.Sprintf(`{"id":"%s","stream":"ordered","type":"First","pos":0,"gpos":10,"data":{},"meta":null,"time":"2025-01-15T10:00:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"ordered","type":"Second","pos":1,"gpos":20,"data":{},"meta":null,"time":"2025-01-15T10:01:00Z"}`, uuid.New().String()),
		fmt.Sprintf(`{"id":"%s","stream":"ordered","type":"Third","pos":2,"gpos":30,"data":{},"meta":null,"time":"2025-01-15T10:02:00Z"}`, uuid.New().String()),
	}
	body := strings.Join(records, "\n")

	req, _ := http.NewRequest("POST", server.URL()+"/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+server.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	// Retrieve and verify positions
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, server.Env.Namespace, "ordered", &store.GetOpts{})
	if err != nil {
		t.Fatalf("Failed to get stream: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}

	expectedPositions := []int64{0, 1, 2}
	for i, msg := range msgs {
		if msg.Position != expectedPositions[i] {
			t.Errorf("Message %d: expected position %d, got %d", i, expectedPositions[i], msg.Position)
		}
	}
}
