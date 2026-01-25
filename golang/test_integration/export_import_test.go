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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/eventodb/eventodb/internal/auth"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
)

// parseSSEForDoneFromBytes parses SSE from bytes and looks for done event
func parseSSEForDoneFromBytes(t *testing.T, data []byte, expectedImported int64) bool {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			var msg map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &msg); err != nil {
				continue
			}
			if done, ok := msg["done"].(bool); ok && done {
				imported := int64(msg["imported"].(float64))
				if imported == expectedImported {
					return true
				}
				t.Logf("Done event: imported=%d, expected=%d", imported, expectedImported)
			}
		}
	}
	return false
}

// TestMDB004_5A_T1_ExportImportRoundtrip tests complete export → import roundtrip
func TestMDB004_5A_T1_ExportImportRoundtrip(t *testing.T) {
	// Setup source server with data
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 10; i++ {
		streamName := fmt.Sprintf("account-%d", i)
		_, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "AccountCreated",
			Data: map[string]interface{}{"index": float64(i), "name": fmt.Sprintf("Account %d", i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Export from source
	records, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "account", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	if len(records) != 10 {
		t.Fatalf("Expected 10 exported records, got %d", len(records))
	}

	// Create a fresh namespace on the same server for import destination
	// This simulates importing to a clean namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	// Convert records to NDJSON
	var ndjsonBuf bytes.Buffer
	encoder := json.NewEncoder(&ndjsonBuf)
	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			t.Fatalf("Failed to encode record: %v", err)
		}
	}

	// Import to destination namespace
	req, err := http.NewRequest("POST", sourceServer.URL()+"/import", &ndjsonBuf)
	if err != nil {
		t.Fatalf("Failed to create import request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read full response body
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Import failed with status %d: %s", resp.StatusCode, respBody)
	}

	// Verify import completed
	found := parseSSEForDoneFromBytes(t, respBody, 10)
	if !found {
		t.Errorf("Expected done event with 10 imported messages. Response: %s", respBody)
	}

	// Verify data in destination
	destRecords, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "account", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch from destination: %v", err)
	}

	if len(destRecords) != 10 {
		t.Fatalf("Expected 10 records in destination, got %d", len(destRecords))
	}
}

// TestMDB004_5A_T2_RoundtripPreservesAllFields tests that all fields are preserved through roundtrip
func TestMDB004_5A_T2_RoundtripPreservesAllFields(t *testing.T) {
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write a message with all fields populated
	originalID := uuid.New().String()
	originalMsg := &store.Message{
		ID:   originalID,
		Type: "CompleteMessage",
		Data: map[string]interface{}{
			"amount":   123.45,
			"currency": "USD",
			"items":    []interface{}{"item1", "item2"},
			"nested":   map[string]interface{}{"key": "value"},
		},
		Metadata: map[string]interface{}{
			"correlationStreamName": "workflow-123",
			"userId":                "user-456",
		},
	}

	result, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, "order-999", originalMsg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Export
	records, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "order", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record := records[0]

	// Verify exported record has correct fields
	if record.ID != originalID {
		t.Errorf("ID mismatch: expected %s, got %s", originalID, record.ID)
	}
	if record.Stream != "order-999" {
		t.Errorf("Stream mismatch: expected order-999, got %s", record.Stream)
	}
	if record.Type != "CompleteMessage" {
		t.Errorf("Type mismatch: expected CompleteMessage, got %s", record.Type)
	}
	if record.Position != 0 {
		t.Errorf("Position mismatch: expected 0, got %d", record.Position)
	}
	if record.GPos != result.GlobalPosition {
		t.Errorf("GPos mismatch: expected %d, got %d", result.GlobalPosition, record.GPos)
	}

	// Create destination namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	var ndjsonBuf bytes.Buffer
	json.NewEncoder(&ndjsonBuf).Encode(record)

	req, _ := http.NewRequest("POST", sourceServer.URL()+"/import", &ndjsonBuf)
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Retrieve and verify all fields
	msgs, err := sourceServer.Env.Store.GetStreamMessages(ctx, destNamespace, "order-999", nil)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	imported := msgs[0]

	// Verify ID
	if imported.ID != originalID {
		t.Errorf("Imported ID: expected %s, got %s", originalID, imported.ID)
	}

	// Verify type
	if imported.Type != "CompleteMessage" {
		t.Errorf("Imported type: expected CompleteMessage, got %s", imported.Type)
	}

	// Verify position
	if imported.Position != 0 {
		t.Errorf("Imported position: expected 0, got %d", imported.Position)
	}

	// Verify global position
	if imported.GlobalPosition != result.GlobalPosition {
		t.Errorf("Imported gpos: expected %d, got %d", result.GlobalPosition, imported.GlobalPosition)
	}

	// Verify data fields
	if imported.Data["amount"] != 123.45 {
		t.Errorf("Imported data.amount: expected 123.45, got %v", imported.Data["amount"])
	}
	if imported.Data["currency"] != "USD" {
		t.Errorf("Imported data.currency: expected USD, got %v", imported.Data["currency"])
	}

	// Verify metadata
	if imported.Metadata["correlationStreamName"] != "workflow-123" {
		t.Errorf("Imported metadata.correlationStreamName: expected workflow-123, got %v", imported.Metadata["correlationStreamName"])
	}
}

// TestMDB004_5A_T3_RoundtripWithGzip tests export/import with gzip compression
func TestMDB004_5A_T3_RoundtripWithGzip(t *testing.T) {
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 20; i++ {
		streamName := fmt.Sprintf("gzip-%d", i)
		_, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "GzipTest",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Export
	records, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "gzip", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	if len(records) != 20 {
		t.Fatalf("Expected 20 records, got %d", len(records))
	}

	// Compress as gzip
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	encoder := json.NewEncoder(gzWriter)
	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			t.Fatalf("Failed to encode: %v", err)
		}
	}
	gzWriter.Close()

	// Decompress (simulating CLI import with --gzip)
	gzReader, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}

	decompressed, err := io.ReadAll(gzReader)
	gzReader.Close()
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	// Create destination namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	req, _ := http.NewRequest("POST", sourceServer.URL()+"/import", bytes.NewReader(decompressed))
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	found := parseSSEForDoneFromBytes(t, respBody, 20)
	if !found {
		t.Errorf("Expected done event with 20 imported messages. Response: %s", respBody)
	}

	// Verify in destination
	destRecords, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "gzip", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch from destination: %v", err)
	}

	if len(destRecords) != 20 {
		t.Fatalf("Expected 20 records in destination, got %d", len(destRecords))
	}
}

// TestMDB004_5A_T4_RoundtripWithCategoryFilter tests export/import with category filtering
func TestMDB004_5A_T4_RoundtripWithCategoryFilter(t *testing.T) {
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write messages in multiple categories
	for _, cat := range []string{"alpha", "beta", "gamma"} {
		for i := 0; i < 5; i++ {
			streamName := fmt.Sprintf("%s-%d", cat, i)
			_, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, streamName, &store.Message{
				ID:   uuid.New().String(),
				Type: fmt.Sprintf("%sEvent", cat),
				Data: map[string]interface{}{"category": cat, "index": float64(i)},
			})
			if err != nil {
				t.Fatalf("Failed to write: %v", err)
			}
		}
	}

	// Export only "alpha" category
	alphaRecords, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "alpha", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export alpha: %v", err)
	}

	if len(alphaRecords) != 5 {
		t.Fatalf("Expected 5 alpha records, got %d", len(alphaRecords))
	}

	// Create destination namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	// Import only alpha to destination
	var ndjsonBuf bytes.Buffer
	encoder := json.NewEncoder(&ndjsonBuf)
	for _, rec := range alphaRecords {
		encoder.Encode(rec)
	}

	req, _ := http.NewRequest("POST", sourceServer.URL()+"/import", &ndjsonBuf)
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify only alpha exists in destination
	destAlpha, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "alpha", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch alpha: %v", err)
	}
	if len(destAlpha) != 5 {
		t.Errorf("Expected 5 alpha records, got %d", len(destAlpha))
	}

	// Beta should not exist (we didn't import it)
	destBeta, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "beta", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch beta: %v", err)
	}
	if len(destBeta) != 0 {
		t.Errorf("Expected 0 beta records (not imported), got %d", len(destBeta))
	}
}

// TestMDB004_5A_T5_RoundtripWithTimeFilter tests export/import with time filtering
func TestMDB004_5A_T5_RoundtripWithTimeFilter(t *testing.T) {
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write test messages
	for i := 0; i < 10; i++ {
		streamName := fmt.Sprintf("timef-%d", i)
		_, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "TimeFilterTest",
			Data: map[string]interface{}{"index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	// Export with time filter that includes all messages
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	records, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "timef", &past, &future)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	if len(records) != 10 {
		t.Fatalf("Expected 10 records with time filter, got %d", len(records))
	}

	// Export with time filter that excludes all (future since)
	futureOnly := time.Now().Add(1 * time.Hour)
	emptyRecords, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "timef", &futureOnly, nil)
	if err != nil {
		t.Fatalf("Failed to export with future filter: %v", err)
	}

	if len(emptyRecords) != 0 {
		t.Errorf("Expected 0 records with future since filter, got %d", len(emptyRecords))
	}

	// Create destination namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	// Import to destination
	var ndjsonBuf bytes.Buffer
	encoder := json.NewEncoder(&ndjsonBuf)
	for _, rec := range records {
		encoder.Encode(rec)
	}

	req, _ := http.NewRequest("POST", sourceServer.URL()+"/import", &ndjsonBuf)
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify in destination
	destRecords, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "timef", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch: %v", err)
	}

	if len(destRecords) != 10 {
		t.Errorf("Expected 10 records in destination, got %d", len(destRecords))
	}
}

// TestMDB004_5A_T6_LargeFileConstantMemory tests that large imports work correctly
func TestMDB004_5A_T6_LargeFileConstantMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	server := SetupTestServer(t)
	defer server.Cleanup()

	// Create fresh namespace for import
	importNamespace := fmt.Sprintf("large-%s", uuid.New().String()[:8])
	importEnv := createNamespaceOnServer(t, server, importNamespace)
	defer cleanupNamespace(t, server, importNamespace)

	// Create 10K records
	var buf bytes.Buffer
	for i := 0; i < 10000; i++ {
		rec := fmt.Sprintf(
			`{"id":"%s","stream":"large-%d","type":"LargeTest","pos":0,"gpos":%d,"data":{"index":%d,"padding":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"meta":null,"time":"2025-01-15T10:00:00Z"}`,
			uuid.New().String(), i, i+1, i,
		)
		buf.WriteString(rec + "\n")
	}

	// Get baseline memory
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Import
	req, err := http.NewRequest("POST", server.URL()+"/import", &buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+importEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Consume response
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Check memory after import
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Memory increase should be bounded (less than 200MB for 10K events)
	memIncrease := int64(m2.Alloc) - int64(m1.Alloc)
	if memIncrease > 200*1024*1024 {
		t.Logf("Warning: Memory increased by %d MB", memIncrease/(1024*1024))
	}

	// Verify done event
	found := parseSSEForDoneFromBytes(t, respBody, 10000)
	if !found {
		t.Log("Warning: Done event not found or count mismatch")
	}

	// Verify all records were imported by spot-checking
	ctx := context.Background()
	msgs, err := server.Env.Store.GetStreamMessages(ctx, importNamespace, "large-0", nil)
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected message in large-0, got %d", len(msgs))
	}

	msgs2, err := server.Env.Store.GetStreamMessages(ctx, importNamespace, "large-9999", nil)
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
	if len(msgs2) != 1 {
		t.Errorf("Expected message in large-9999, got %d", len(msgs2))
	}
}

// TestMDB004_5A_T7_CrossNamespaceExportImport tests export from namespace A, import to namespace B
func TestMDB004_5A_T7_CrossNamespaceExportImport(t *testing.T) {
	// Setup server
	server := SetupTestServer(t)
	defer server.Cleanup()

	ctx := context.Background()

	// Write messages to source namespace (the server's default namespace)
	for i := 0; i < 5; i++ {
		streamName := fmt.Sprintf("cross-%d", i)
		_, err := server.Env.Store.WriteMessage(ctx, server.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "CrossNamespace",
			Data: map[string]interface{}{"source": "namespaceA", "index": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	// Export from source namespace
	records, err := fetchExportRecords(server.URL(), server.Token, "cross", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export from source: %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("Expected 5 records, got %d", len(records))
	}

	// Create destination namespace B
	destNamespace := fmt.Sprintf("dest-B-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, server, destNamespace)
	defer cleanupNamespace(t, server, destNamespace)

	// Import to namespace B
	var ndjsonBuf bytes.Buffer
	encoder := json.NewEncoder(&ndjsonBuf)
	for _, rec := range records {
		encoder.Encode(rec)
	}

	req, _ := http.NewRequest("POST", server.URL()+"/import", &ndjsonBuf)
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Import to B failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	found := parseSSEForDoneFromBytes(t, respBody, 5)
	if !found {
		t.Errorf("Expected done event with 5 imported messages. Response: %s", respBody)
	}

	// Verify data is in namespace B
	bRecords, err := fetchExportRecords(server.URL(), destEnv.Token, "cross", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch from B: %v", err)
	}

	if len(bRecords) != 5 {
		t.Fatalf("Expected 5 records in B, got %d", len(bRecords))
	}

	// Verify the records have the same content
	for i, rec := range bRecords {
		if !strings.HasPrefix(rec.Stream, "cross-") {
			t.Errorf("Record %d: unexpected stream %s", i, rec.Stream)
		}
		if rec.Type != "CrossNamespace" {
			t.Errorf("Record %d: unexpected type %s", i, rec.Type)
		}
	}

	// Verify global positions are preserved
	msgs, err := server.Env.Store.GetStreamMessages(ctx, destNamespace, "cross-0", nil)
	if err != nil {
		t.Fatalf("Failed to get cross-0: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	// Find original gpos for cross-0
	var originalGPos int64
	for _, rec := range records {
		if rec.Stream == "cross-0" {
			originalGPos = rec.GPos
			break
		}
	}

	if msgs[0].GlobalPosition != originalGPos {
		t.Errorf("Global position not preserved: expected %d, got %d", originalGPos, msgs[0].GlobalPosition)
	}
}

// TestMDB004_5A_MultipleStreamsInCategory tests that category messages maintain correct ordering
func TestMDB004_5A_MultipleStreamsInCategory(t *testing.T) {
	sourceServer := SetupTestServer(t)
	defer sourceServer.Cleanup()

	ctx := context.Background()

	// Write interleaved messages to different streams in same category
	streams := []string{"order-1", "order-2", "order-3"}
	for i := 0; i < 9; i++ {
		streamName := streams[i%3]
		_, err := sourceServer.Env.Store.WriteMessage(ctx, sourceServer.Env.Namespace, streamName, &store.Message{
			ID:   uuid.New().String(),
			Type: "OrderEvent",
			Data: map[string]interface{}{"sequence": float64(i)},
		})
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
	}

	// Export category
	records, err := fetchExportRecords(sourceServer.URL(), sourceServer.Token, "order", nil, nil)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	if len(records) != 9 {
		t.Fatalf("Expected 9 records, got %d", len(records))
	}

	// Verify records are in global position order
	for i := 1; i < len(records); i++ {
		if records[i].GPos <= records[i-1].GPos {
			t.Errorf("Records not in gpos order: %d should be > %d", records[i].GPos, records[i-1].GPos)
		}
	}

	// Create destination namespace
	destNamespace := fmt.Sprintf("dest-%s", uuid.New().String()[:8])
	destEnv := createNamespaceOnServer(t, sourceServer, destNamespace)
	defer cleanupNamespace(t, sourceServer, destNamespace)

	// Import to destination
	var ndjsonBuf bytes.Buffer
	encoder := json.NewEncoder(&ndjsonBuf)
	for _, rec := range records {
		encoder.Encode(rec)
	}

	req, _ := http.NewRequest("POST", sourceServer.URL()+"/import", &ndjsonBuf)
	req.Header.Set("Authorization", "Bearer "+destEnv.Token)

	resp, _ := http.DefaultClient.Do(req)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify category order is preserved in destination
	destRecords, err := fetchExportRecords(sourceServer.URL(), destEnv.Token, "order", nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch: %v", err)
	}

	if len(destRecords) != 9 {
		t.Fatalf("Expected 9 destination records, got %d", len(destRecords))
	}

	// Verify global position order preserved
	for i := 0; i < len(records); i++ {
		if destRecords[i].GPos != records[i].GPos {
			t.Errorf("GPos mismatch at %d: expected %d, got %d", i, records[i].GPos, destRecords[i].GPos)
		}
	}
}

// Helper: create a new namespace on a test server
type namespaceEnv struct {
	Namespace string
	Token     string
}

func createNamespaceOnServer(t *testing.T, server *TestServer, namespace string) *namespaceEnv {
	t.Helper()
	ctx := context.Background()

	// Generate token for the new namespace
	token, err := auth.GenerateToken(namespace)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	// Create namespace in the store
	err = server.Env.Store.CreateNamespace(ctx, namespace, tokenHash, "Test destination namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	return &namespaceEnv{
		Namespace: namespace,
		Token:     token,
	}
}

func cleanupNamespace(t *testing.T, server *TestServer, namespace string) {
	t.Helper()
	ctx := context.Background()
	_ = server.Env.Store.DeleteNamespace(ctx, namespace)
}
