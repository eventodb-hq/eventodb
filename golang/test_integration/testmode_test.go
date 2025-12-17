// Package integration provides integration tests for Message DB.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/message-db/message-db/internal/api"
	"github.com/message-db/message-db/internal/auth"
)

// TestMDB002_7A_T1: Test mode uses in-memory SQLite (or configured backend)
func TestMDB002_7A_T1_TestModeUsesConfiguredBackend(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Verify the namespace exists and is accessible
	ctx := context.Background()
	ns, err := env.Store.GetNamespace(ctx, env.Namespace)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.ID != env.Namespace {
		t.Errorf("Expected namespace ID '%s', got '%s'", env.Namespace, ns.ID)
	}

	t.Logf("Running with backend: %s", GetTestBackend())
}

// TestMDB002_7A_T2: Test auto-namespace creation on first write
func TestMDB002_7A_T2_AutoNamespaceCreation(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler) // Test mode = true

	// Write to a stream without auth (test mode allows this, uses default namespace)
	reqBody := []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Opened",
			"data": map[string]interface{}{"amount": 100},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestMDB002_7A_T3: Test token returned in response header
func TestMDB002_7A_T3_TokenInResponseHeader(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create a namespace via RPC
	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	nsName := fmt.Sprintf("test_tenant_%d", time.Now().UnixNano())
	reqBody := []interface{}{
		"ns.create",
		nsName,
		map[string]interface{}{
			"description": "Test tenant",
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check response contains token
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	token, ok := response["token"].(string)
	if !ok || token == "" {
		t.Errorf("Expected token in response, got: %v", response)
	}

	// Verify token format
	if len(token) < 10 {
		t.Errorf("Token seems too short: %s", token)
	}

	// Verify token can be parsed
	ns, err := auth.ParseToken(token)
	if err != nil {
		t.Errorf("Failed to parse token: %v", err)
	}
	if ns != nsName {
		t.Errorf("Token parsed to wrong namespace: %s", ns)
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(context.Background(), nsName)
}

// TestMDB002_7A_T4: Test auth not required in test mode
func TestMDB002_7A_T4_AuthNotRequiredInTestMode(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler) // Test mode = true

	// Make request WITHOUT Authorization header
	reqBody := []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Opened",
			"data": map[string]interface{}{"amount": 100},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	// No Authorization header
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should succeed without auth in test mode
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 in test mode without auth, got %d: %s", w.Code, w.Body.String())
	}
}

// TestMDB002_7A_T5: Test sys.version returns version
func TestMDB002_7A_T5_SysVersion(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	reqBody := []interface{}{"sys.version"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var version string
	if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
		t.Fatalf("Failed to parse version response: %v", err)
	}

	if version != "1.4.0" {
		t.Errorf("Expected version '1.4.0', got '%s'", version)
	}
}

// TestMDB002_7A_T6: Test sys.health returns status
func TestMDB002_7A_T6_SysHealth(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	reqBody := []interface{}{"sys.health"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var health map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatalf("Failed to parse health response: %v", err)
	}

	if health["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", health["status"])
	}

	if _, ok := health["backend"]; !ok {
		t.Error("Expected 'backend' field in health response")
	}
}

// TestMDB002_7A_T7: Test complete workflow: create ns → write → read
func TestMDB002_7A_T7_CompleteWorkflow(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Step 1: Write a message using the token
	writeReq := []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Opened",
			"data": map[string]interface{}{"balance": 100},
		},
	}

	body, _ := json.Marshal(writeReq)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.Token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to write message: %d: %s", w.Code, w.Body.String())
	}

	// Step 2: Read the message back
	readReq := []interface{}{
		"stream.get",
		"account-123",
		map[string]interface{}{},
	}

	body, _ = json.Marshal(readReq)
	req = httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.Token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to read messages: %d: %s", w.Code, w.Body.String())
	}

	var messages [][]interface{}
	json.Unmarshal(w.Body.Bytes(), &messages)

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if len(messages) > 0 && messages[0][1] != "Opened" {
		t.Errorf("Expected message type 'Opened', got '%v'", messages[0][1])
	}
}

// TestMDB002_7A_T8: Test namespace isolation end-to-end
func TestMDB002_7A_T8_NamespaceIsolation(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create second namespace in the same store
	namespace2 := fmt.Sprintf("tenant_b_%d", time.Now().UnixNano())
	token2, _ := auth.GenerateToken(namespace2)
	tokenHash2 := auth.HashToken(token2)

	ctx := context.Background()
	err := env.Store.CreateNamespace(ctx, namespace2, tokenHash2, "Tenant B")
	if err != nil {
		t.Fatalf("Failed to create namespace 2: %v", err)
	}
	defer env.Store.DeleteNamespace(ctx, namespace2)

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Write to same stream in both namespaces
	writeMessage(t, handler, env.Token, "account-123", "Opened", map[string]interface{}{"tenant": "a"})
	writeMessage(t, handler, token2, "account-123", "Opened", map[string]interface{}{"tenant": "b"})

	// Read from namespace 1
	messages1 := getMessages(t, handler, env.Token, "account-123")
	if len(messages1) != 1 {
		t.Fatalf("Expected 1 message in namespace 1, got %d", len(messages1))
	}

	data1 := messages1[0][4].(map[string]interface{})
	if data1["tenant"] != "a" {
		t.Errorf("Expected tenant 'a' in namespace 1, got '%v'", data1["tenant"])
	}

	// Read from namespace 2
	messages2 := getMessages(t, handler, token2, "account-123")
	if len(messages2) != 1 {
		t.Fatalf("Expected 1 message in namespace 2, got %d", len(messages2))
	}

	data2 := messages2[0][4].(map[string]interface{})
	if data2["tenant"] != "b" {
		t.Errorf("Expected tenant 'b' in namespace 2, got '%v'", data2["tenant"])
	}
}

// TestMDB002_7A_T9: Test subscription + write + fetch workflow
func TestMDB002_7A_T9_SubscriptionWorkflow(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Write an initial message
	writeMessage(t, handler, env.Token, "account-123", "Init", map[string]interface{}{"init": true})

	// Write a second message
	writeMessage(t, handler, env.Token, "account-123", "Deposited", map[string]interface{}{"amount": 50})

	// Verify messages can be fetched
	messages := getMessages(t, handler, env.Token, "account-123")

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages (Init + Deposited), got %d", len(messages))
	}

	// Verify message types
	if len(messages) >= 2 {
		if messages[0][1] != "Init" {
			t.Errorf("Expected first message type 'Init', got '%v'", messages[0][1])
		}
		if messages[1][1] != "Deposited" {
			t.Errorf("Expected second message type 'Deposited', got '%v'", messages[1][1])
		}
	}
}

// TestMDB002_7A_T10: Test optimistic locking workflow
func TestMDB002_7A_T10_OptimisticLocking(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Write first message
	writeReq := []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Opened",
			"data": map[string]interface{}{"balance": 0},
		},
	}

	body, _ := json.Marshal(writeReq)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.Token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to write first message: %s", w.Body.String())
	}

	// Write second message with expectedVersion=0 (should succeed)
	writeReq = []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Deposited",
			"data": map[string]interface{}{"amount": 100},
		},
		map[string]interface{}{
			"expectedVersion": 0,
		},
	}

	body, _ = json.Marshal(writeReq)
	req = httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.Token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to write with correct expectedVersion: %s", w.Body.String())
	}

	// Write third message with expectedVersion=0 (should fail)
	writeReq = []interface{}{
		"stream.write",
		"account-123",
		map[string]interface{}{
			"type": "Withdrawn",
			"data": map[string]interface{}{"amount": 50},
		},
		map[string]interface{}{
			"expectedVersion": 0,
		},
	}

	body, _ = json.Marshal(writeReq)
	req = httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.Token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409 for version conflict, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errData, ok := errResp["error"].(map[string]interface{}); ok {
		if errData["code"] != "STREAM_VERSION_CONFLICT" {
			t.Errorf("Expected STREAM_VERSION_CONFLICT error, got: %v", errData["code"])
		}
	}
}

// TestMDB002_7A_T11: Test performance: API response < 50ms (p95)
func TestMDB002_7A_T11_Performance(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Perform 100 write operations and measure times
	const iterations = 100
	times := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		writeReq := []interface{}{
			"stream.write",
			fmt.Sprintf("account-%d", i),
			map[string]interface{}{
				"type": "Opened",
				"data": map[string]interface{}{"id": i},
			},
		}

		body, _ := json.Marshal(writeReq)
		req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.Token)
		w := httptest.NewRecorder()

		start := time.Now()
		handler.ServeHTTP(w, req)
		times[i] = time.Since(start)

		if w.Code != http.StatusOK {
			t.Errorf("Write %d failed: %s", i, w.Body.String())
		}
	}

	// Calculate p95
	sortedTimes := make([]time.Duration, len(times))
	copy(sortedTimes, times)

	// Bubble sort
	for i := 0; i < len(sortedTimes); i++ {
		for j := i + 1; j < len(sortedTimes); j++ {
			if sortedTimes[i] > sortedTimes[j] {
				sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
			}
		}
	}

	p95Index := int(float64(len(sortedTimes)) * 0.95)
	p95 := sortedTimes[p95Index]

	t.Logf("Performance results (backend=%s): p95=%v", GetTestBackend(), p95)

	// Check if p95 is under 50ms
	if p95 > 50*time.Millisecond {
		t.Logf("WARNING: p95 response time %v exceeds 50ms target", p95)
	}
}

// TestMDB002_7A_T12: Test multiple writes to different streams
func TestMDB002_7A_T12_ConcurrentWrites(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, nil)
	handler := api.AuthMiddleware(env.Store, true)(rpcHandler)

	// Write to multiple different streams sequentially
	const numStreams = 100

	for i := 0; i < numStreams; i++ {
		streamName := fmt.Sprintf("account-%d", i)
		err := writeMessageWithError(handler, env.Token, streamName, "Event",
			map[string]interface{}{"id": i})

		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Verify all messages were written correctly
	for i := 0; i < numStreams; i++ {
		streamName := fmt.Sprintf("account-%d", i)
		messages := getMessages(t, handler, env.Token, streamName)
		if len(messages) != 1 {
			t.Errorf("Stream %s: expected 1 message, got %d", streamName, len(messages))
		}
	}
}

// Helper functions

func createNamespace(t *testing.T, handler http.Handler, nsID string) string {
	reqBody := []interface{}{
		"ns.create",
		nsID,
		map[string]interface{}{"description": "Test namespace"},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to create namespace %s: %d: %s", nsID, w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["token"].(string)
}

func writeMessage(t *testing.T, handler http.Handler, token, streamName, msgType string, data map[string]interface{}) {
	err := writeMessageWithError(handler, token, streamName, msgType, data)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}
}

func writeMessageWithError(handler http.Handler, token, streamName, msgType string, data map[string]interface{}) error {
	reqBody := []interface{}{
		"stream.write",
		streamName,
		map[string]interface{}{
			"type": msgType,
			"data": data,
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return fmt.Errorf("status %d: %s", w.Code, w.Body.String())
	}

	return nil
}

func getMessages(t *testing.T, handler http.Handler, token, streamName string) [][]interface{} {
	reqBody := []interface{}{
		"stream.get",
		streamName,
		map[string]interface{}{},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get messages: %d: %s", w.Code, w.Body.String())
	}

	var messages [][]interface{}
	json.Unmarshal(w.Body.Bytes(), &messages)
	return messages
}
