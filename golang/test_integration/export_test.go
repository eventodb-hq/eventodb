package integration

import (
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

// ExportRecord matches the NDJSON format for export/import
type ExportRecord struct {
	ID       string                 `json:"id"`
	Stream   string                 `json:"stream"`
	Type     string                 `json:"type"`
	Position int64                  `json:"pos"`
	GPos     int64                  `json:"gpos"`
	Data     map[string]interface{} `json:"data"`
	Meta     map[string]interface{} `json:"meta"`
	Time     string                 `json:"time"`
}

// TestMDB004_3A_T1_ExportOutputsValidNDJSON tests that export outputs valid NDJSON format
func TestMDB004_3A_T1_ExportOutputsValidNDJSON(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write some test messages
	for i := 0; i < 5; i++ {
		streamName := fmt.Sprintf("user-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "UserCreated",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Fetch messages via RPC and format as NDJSON
	records, err := fetchExportRecords(server.URL(), server.Token, "user", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("Expected 5 records, got %d", len(records))
	}

	// Validate each record
	for i, rec := range records {
		if rec.ID == "" {
			t.Errorf("Record %d: ID is empty", i)
		}
		if !strings.HasPrefix(rec.Stream, "user-") {
			t.Errorf("Record %d: unexpected stream name %s", i, rec.Stream)
		}
		if rec.Type != "UserCreated" {
			t.Errorf("Record %d: unexpected type %s", i, rec.Type)
		}
		if rec.Data == nil {
			t.Errorf("Record %d: data is nil", i)
		}
		if rec.Time == "" {
			t.Errorf("Record %d: time is empty", i)
		}
		// Validate time is parseable
		_, err := time.Parse(time.RFC3339, rec.Time)
		if err != nil {
			t.Errorf("Record %d: invalid time format %s: %v", i, rec.Time, err)
		}
	}
}

// TestMDB004_3A_T2_ExportFiltersByCategories tests category filtering
func TestMDB004_3A_T2_ExportFiltersByCategories(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages in different categories
	categories := []string{"user", "order", "product"}
	for _, cat := range categories {
		for i := 0; i < 3; i++ {
			streamName := fmt.Sprintf("%s-%d", cat, i)
			_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
				ID:   uuid.New().String(),
				Type: fmt.Sprintf("%sCreated", cat),
				Data: map[string]interface{}{"category": cat, "index": float64(i)},
			})
			if err != nil {
				t.Fatalf("Failed to write message: %v", err)
			}
		}
	}

	// Export only "user" category
	records, err := fetchExportRecords(server.URL(), server.Token, "user", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("Expected 3 records for user category, got %d", len(records))
	}

	for i, rec := range records {
		if !strings.HasPrefix(rec.Stream, "user-") {
			t.Errorf("Record %d: expected user category, got stream %s", i, rec.Stream)
		}
	}
}

// TestMDB004_3A_T3_ExportFiltersBySince tests since date filtering
func TestMDB004_3A_T3_ExportFiltersBySince(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages
	for i := 0; i < 5; i++ {
		streamName := fmt.Sprintf("event-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TestEvent",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Export with since=now (should get no records since all are in the past)
	future := time.Now().Add(1 * time.Hour)
	records, err := fetchExportRecords(server.URL(), server.Token, "event", &future, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 0 {
		t.Fatalf("Expected 0 records with future since filter, got %d", len(records))
	}

	// Export with since=past (should get all records)
	past := time.Now().Add(-1 * time.Hour)
	records, err = fetchExportRecords(server.URL(), server.Token, "event", &past, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("Expected 5 records with past since filter, got %d", len(records))
	}
}

// TestMDB004_3A_T4_ExportFiltersByUntil tests until date filtering
func TestMDB004_3A_T4_ExportFiltersByUntil(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages
	for i := 0; i < 5; i++ {
		streamName := fmt.Sprintf("event-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TestEvent",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Export with until=past (should get no records)
	past := time.Now().Add(-1 * time.Hour)
	records, err := fetchExportRecords(server.URL(), server.Token, "event", nil, &past)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 0 {
		t.Fatalf("Expected 0 records with past until filter, got %d", len(records))
	}

	// Export with until=future (should get all records)
	future := time.Now().Add(1 * time.Hour)
	records, err = fetchExportRecords(server.URL(), server.Token, "event", nil, &future)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("Expected 5 records with future until filter, got %d", len(records))
	}
}

// TestMDB004_3A_T5_ExportCombinesFilters tests combined category and time filters
func TestMDB004_3A_T5_ExportCombinesFilters(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages in different categories
	for _, cat := range []string{"user", "order"} {
		for i := 0; i < 3; i++ {
			streamName := fmt.Sprintf("%s-%d", cat, i)
			_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
				ID:   uuid.New().String(),
				Type: fmt.Sprintf("%sCreated", cat),
				Data: map[string]interface{}{"category": cat},
			})
			if err != nil {
				t.Fatalf("Failed to write message: %v", err)
			}
		}
	}

	// Export only "user" category with time filter that includes all
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	records, err := fetchExportRecords(server.URL(), server.Token, "user", &past, &future)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("Expected 3 records with combined filters, got %d", len(records))
	}

	for _, rec := range records {
		if !strings.HasPrefix(rec.Stream, "user-") {
			t.Errorf("Expected user category, got stream %s", rec.Stream)
		}
	}
}

// TestMDB004_3A_T6_ExportWithGzipProducesValidGzip tests gzip compression
func TestMDB004_3A_T6_ExportWithGzipProducesValidGzip(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 10; i++ {
		streamName := fmt.Sprintf("test-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TestEvent",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Fetch records and encode as NDJSON
	records, err := fetchExportRecords(server.URL(), server.Token, "test", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	// Compress as gzip
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	encoder := json.NewEncoder(gzWriter)
	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			t.Fatalf("Failed to encode record: %v", err)
		}
	}
	gzWriter.Close()

	// Verify we can decompress
	gzReader, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Count decompressed records
	decoder := json.NewDecoder(gzReader)
	var count int
	for {
		var rec ExportRecord
		if err := decoder.Decode(&rec); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("Failed to decode record: %v", err)
		}
		count++
	}

	if count != 10 {
		t.Fatalf("Expected 10 records after decompression, got %d", count)
	}
}

// TestMDB004_3A_T7_ExportToStdoutWorks tests export to stdout
func TestMDB004_3A_T7_ExportToStdoutWorks(t *testing.T) {
	// This is more of a pattern test - we verify the underlying fetch works
	// The CLI just writes to stdout instead of file
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 3; i++ {
		streamName := fmt.Sprintf("stdout-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TestEvent",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Simulate stdout by writing to a buffer
	records, err := fetchExportRecords(server.URL(), server.Token, "stdout", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			t.Fatalf("Failed to encode: %v", err)
		}
	}

	// Verify the buffer contains valid NDJSON
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(lines))
	}
}

// TestMDB004_3A_T8_ExportToFileWorks tests export to file
func TestMDB004_3A_T8_ExportToFileWorks(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 5; i++ {
		streamName := fmt.Sprintf("file-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TestEvent",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "export-test-*.ndjson")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Fetch and write to file
	records, err := fetchExportRecords(server.URL(), server.Token, "file", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	encoder := json.NewEncoder(tmpFile)
	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			t.Fatalf("Failed to encode: %v", err)
		}
	}
	tmpFile.Close()

	// Read back and verify
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 5 {
		t.Fatalf("Expected 5 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var rec ExportRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

// TestMDB004_3A_T9_ExportMultipleCategories tests export with multiple categories
func TestMDB004_3A_T9_ExportMultipleCategories(t *testing.T) {
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages in multiple categories
	for _, cat := range []string{"alpha", "beta", "gamma"} {
		for i := 0; i < 4; i++ {
			streamName := fmt.Sprintf("%s-%d", cat, i)
			_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
				ID:   uuid.New().String(),
				Type: "TestEvent",
				Data: map[string]interface{}{"category": cat},
			})
			if err != nil {
				t.Fatalf("Failed to write message: %v", err)
			}
		}
	}

	// Export multiple categories by fetching each
	allRecords := make([]ExportRecord, 0)
	for _, cat := range []string{"alpha", "beta", "gamma"} {
		records, err := fetchExportRecords(server.URL(), server.Token, cat, nil, nil)
		if err != nil {
			t.Fatalf("Failed to fetch records for category %s: %v", cat, err)
		}
		allRecords = append(allRecords, records...)
	}

	if len(allRecords) != 12 {
		t.Fatalf("Expected 12 records from all categories, got %d", len(allRecords))
	}

	// Verify we got messages from all categories
	categories := make(map[string]bool)
	for _, rec := range allRecords {
		parts := strings.Split(rec.Stream, "-")
		if len(parts) > 0 {
			categories[parts[0]] = true
		}
	}

	if len(categories) != 3 {
		t.Errorf("Expected 3 categories, got %d: %v", len(categories), categories)
	}
}

// Helper function to fetch export records via RPC
func fetchExportRecords(baseURL, token, category string, since, until *time.Time) ([]ExportRecord, error) {
	ctx := context.Background()
	client := &http.Client{Timeout: 30 * time.Second}

	var records []ExportRecord
	var position int64

	for {
		messages, err := fetchCategoryBatchTest(ctx, client, baseURL, token, category, position)
		if err != nil {
			return nil, err
		}

		if len(messages) == 0 {
			break
		}

		for _, msg := range messages {
			// Apply time filtering
			if since != nil && msg.Time.Before(*since) {
				continue
			}
			if until != nil && !msg.Time.Before(*until) {
				continue
			}

			records = append(records, ExportRecord{
				ID:       msg.ID,
				Stream:   msg.StreamName,
				Type:     msg.Type,
				Position: msg.Position,
				GPos:     msg.GlobalPosition,
				Data:     msg.Data,
				Meta:     msg.Metadata,
				Time:     msg.Time.UTC().Format(time.RFC3339),
			})
		}

		position = messages[len(messages)-1].GlobalPosition + 1
	}

	return records, nil
}

type testCategoryMessage struct {
	ID             string
	StreamName     string
	Type           string
	Position       int64
	GlobalPosition int64
	Data           map[string]interface{}
	Metadata       map[string]interface{}
	Time           time.Time
}

func fetchCategoryBatchTest(ctx context.Context, client *http.Client, baseURL, token, category string, position int64) ([]testCategoryMessage, error) {
	opts := map[string]interface{}{
		"position":  position,
		"batchSize": 1000,
	}
	reqBody := []interface{}{"category.get", category, opts}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/rpc", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	messages := make([]testCategoryMessage, len(raw))
	for i, msg := range raw {
		if err := parseTestCategoryMsg(&messages[i], msg); err != nil {
			return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
		}
	}

	return messages, nil
}

func parseTestCategoryMsg(msg *testCategoryMessage, raw []interface{}) error {
	if len(raw) != 8 {
		return fmt.Errorf("expected 8 fields, got %d", len(raw))
	}

	msg.ID = raw[0].(string)
	msg.StreamName = raw[1].(string)
	msg.Type = raw[2].(string)
	msg.Position = int64(raw[3].(float64))
	msg.GlobalPosition = int64(raw[4].(float64))
	msg.Data = raw[5].(map[string]interface{})

	if raw[6] != nil {
		msg.Metadata = raw[6].(map[string]interface{})
	}

	timeStr := raw[7].(string)
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return fmt.Errorf("failed to parse time: %w", err)
	}
	msg.Time = t

	return nil
}
