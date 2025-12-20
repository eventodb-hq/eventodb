package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventodb/eventodb/internal/api"
	"github.com/eventodb/eventodb/internal/auth"
)

// Helper function to make RPC calls directly to handler
func makeDirectRPCCall(t *testing.T, handler http.Handler, method string, args ...interface{}) (interface{}, map[string]interface{}) {
	t.Helper()

	// Build request
	reqBody := []interface{}{method}
	reqBody = append(reqBody, args...)

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest("POST", "/rpc", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	// Make request
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Parse response
	var result interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check if it's an error response
	if resultMap, ok := result.(map[string]interface{}); ok {
		if errObj, hasError := resultMap["error"]; hasError {
			return nil, errObj.(map[string]interface{})
		}
	}

	return result, nil
}

// Test MDB002_5A_T1: Test create namespace returns token
func TestMDB002_5A_T1_CreateNamespaceReturnsToken(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)

	// Test
	result, errResult := makeDirectRPCCall(t, handler, "ns.create", "tenant_a")
	if errResult != nil {
		t.Fatalf("Expected success, got error: %v", errResult)
	}

	// Verify result
	resultMap := result.(map[string]interface{})
	if resultMap["namespace"] != "tenant_a" {
		t.Errorf("Expected namespace 'tenant_a', got '%v'", resultMap["namespace"])
	}

	token, ok := resultMap["token"].(string)
	if !ok || token == "" {
		t.Error("Expected non-empty token")
	}

	// Verify token format
	if !strings.HasPrefix(token, "ns_") {
		t.Errorf("Token should start with 'ns_', got: %s", token)
	}

	// Verify token can be parsed
	ns, err := auth.ParseToken(token)
	if err != nil {
		t.Errorf("Failed to parse token: %v", err)
	}
	if ns != "tenant_a" {
		t.Errorf("Token parsed to wrong namespace: %s", ns)
	}

	// Verify createdAt is present
	if _, ok := resultMap["createdAt"].(string); !ok {
		t.Error("Expected createdAt timestamp")
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(context.Background(), "tenant_a")
}

// Test MDB002_5A_T2: Test namespace schema/database created
func TestMDB002_5A_T2_NamespaceSchemaCreated(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace
	result, errResult := makeDirectRPCCall(t, handler, "ns.create", "tenant_b")
	if errResult != nil {
		t.Fatalf("Expected success, got error: %v", errResult)
	}

	// Verify namespace exists in store
	ns, err := env.Store.GetNamespace(ctx, "tenant_b")
	if err != nil {
		t.Fatalf("Namespace not found in store: %v", err)
	}

	if ns.ID != "tenant_b" {
		t.Errorf("Expected namespace ID 'tenant_b', got '%s'", ns.ID)
	}

	// Verify token hash is stored
	resultMap := result.(map[string]interface{})
	token := resultMap["token"].(string)
	expectedHash := auth.HashToken(token)

	if ns.TokenHash != expectedHash {
		t.Error("Token hash mismatch in stored namespace")
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(ctx, "tenant_b")
}

// Test MDB002_5A_T3: Test duplicate namespace returns NAMESPACE_EXISTS
func TestMDB002_5A_T3_DuplicateNamespaceReturnsError(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace
	_, errResult := makeDirectRPCCall(t, handler, "ns.create", "duplicate_test")
	if errResult != nil {
		t.Fatalf("First create should succeed: %v", errResult)
	}

	// Try to create same namespace again
	_, errResult = makeDirectRPCCall(t, handler, "ns.create", "duplicate_test")
	if errResult == nil {
		t.Fatal("Expected error for duplicate namespace")
	}

	// Verify error code
	if errResult["code"] != "NAMESPACE_EXISTS" {
		t.Errorf("Expected error code 'NAMESPACE_EXISTS', got '%v'", errResult["code"])
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(ctx, "duplicate_test")
}

// Test MDB002_5A_T4: Test delete namespace removes all data
func TestMDB002_5A_T4_DeleteNamespaceRemovesAllData(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace
	result, _ := makeDirectRPCCall(t, handler, "ns.create", "to_delete")
	token := result.(map[string]interface{})["token"].(string)
	tokenHash := auth.HashToken(token)

	// Verify namespace exists
	_, err := env.Store.GetNamespace(ctx, "to_delete")
	if err != nil {
		t.Fatalf("Namespace should exist: %v", err)
	}

	// Delete namespace
	delResult, errResult := makeDirectRPCCall(t, handler, "ns.delete", "to_delete")
	if errResult != nil {
		t.Fatalf("Expected successful deletion, got error: %v", errResult)
	}

	// Verify delete result
	delMap := delResult.(map[string]interface{})
	if delMap["namespace"] != "to_delete" {
		t.Errorf("Expected namespace 'to_delete', got '%v'", delMap["namespace"])
	}

	if _, ok := delMap["deletedAt"].(string); !ok {
		t.Error("Expected deletedAt timestamp")
	}

	// Verify namespace no longer exists
	_, err = env.Store.GetNamespace(ctx, "to_delete")
	if err == nil {
		t.Error("Namespace should not exist after deletion")
	}

	// Verify the token is no longer valid by checking hash
	namespaces, _ := env.Store.ListNamespaces(ctx)
	for _, ns := range namespaces {
		if ns.TokenHash == tokenHash {
			t.Error("Token hash should be removed from all namespaces")
		}
	}
}

// Test MDB002_5A_T5: Test delete invalidates token
func TestMDB002_5A_T5_DeleteInvalidatesToken(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace
	result, _ := makeDirectRPCCall(t, handler, "ns.create", "token_test")
	token := result.(map[string]interface{})["token"].(string)

	// Verify namespace can be accessed
	_, err := env.Store.GetNamespace(ctx, "token_test")
	if err != nil {
		t.Fatalf("Namespace should exist before deletion: %v", err)
	}

	// Delete namespace
	_, errResult := makeDirectRPCCall(t, handler, "ns.delete", "token_test")
	if errResult != nil {
		t.Fatalf("Delete should succeed: %v", errResult)
	}

	// Verify namespace is gone
	_, err = env.Store.GetNamespace(ctx, "token_test")
	if err == nil {
		t.Error("Namespace should not exist after deletion")
	}

	// Parse token - should still parse (token format is valid)
	// but the namespace it refers to no longer exists
	ns, err := auth.ParseToken(token)
	if err != nil {
		t.Errorf("Token parsing should still work: %v", err)
	}
	if ns != "token_test" {
		t.Errorf("Token should still parse to 'token_test', got '%s'", ns)
	}

	// But the namespace shouldn't exist
	_, err = env.Store.GetNamespace(ctx, ns)
	if err == nil {
		t.Error("Namespace from token should not exist")
	}
}

// Test MDB002_5A_T6: Test delete with wrong namespace fails
func TestMDB002_5A_T6_DeleteNonexistentNamespaceFails(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)

	// Try to delete non-existent namespace
	_, errResult := makeDirectRPCCall(t, handler, "ns.delete", "nonexistent")
	if errResult == nil {
		t.Fatal("Expected error when deleting non-existent namespace")
	}

	// Verify error code
	if errResult["code"] != "NAMESPACE_NOT_FOUND" {
		t.Errorf("Expected error code 'NAMESPACE_NOT_FOUND', got '%v'", errResult["code"])
	}
}

// Test MDB002_5A_T7: Test list returns all namespaces
func TestMDB002_5A_T7_ListReturnsAllNamespaces(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create multiple namespaces
	namespaces := []string{"ns_1", "ns_2", "ns_3"}
	for _, ns := range namespaces {
		_, errResult := makeDirectRPCCall(t, handler, "ns.create", ns)
		if errResult != nil {
			t.Fatalf("Failed to create namespace %s: %v", ns, errResult)
		}
	}

	// List namespaces
	result, errResult := makeDirectRPCCall(t, handler, "ns.list")
	if errResult != nil {
		t.Fatalf("Expected successful list, got error: %v", errResult)
	}

	// Verify result is an array
	resultList, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected array result, got %T", result)
	}

	// Verify count (at least our created namespaces + the env namespace)
	expectedMinCount := len(namespaces) + 1 // +1 for env.Namespace
	if len(resultList) < expectedMinCount {
		t.Errorf("Expected at least %d namespaces, got %d", expectedMinCount, len(resultList))
	}

	// Verify each namespace is in the list
	nsMap := make(map[string]bool)
	for _, item := range resultList {
		itemMap := item.(map[string]interface{})
		nsMap[itemMap["namespace"].(string)] = true

		// Verify required fields
		if _, ok := itemMap["description"]; !ok {
			t.Error("Missing description field")
		}
		if _, ok := itemMap["createdAt"]; !ok {
			t.Error("Missing createdAt field")
		}
		if _, ok := itemMap["messageCount"]; !ok {
			t.Error("Missing messageCount field")
		}
	}

	for _, ns := range namespaces {
		if !nsMap[ns] {
			t.Errorf("Namespace '%s' not in list", ns)
		}
	}

	// Cleanup
	for _, ns := range namespaces {
		_ = env.Store.DeleteNamespace(ctx, ns)
	}
}

// Test MDB002_5A_T8: Test list includes message counts
func TestMDB002_5A_T8_ListIncludesMessageCounts(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace
	_, errResult := makeDirectRPCCall(t, handler, "ns.create", "count_test")
	if errResult != nil {
		t.Fatalf("Failed to create namespace: %v", errResult)
	}

	// List namespaces
	result, errResult := makeDirectRPCCall(t, handler, "ns.list")
	if errResult != nil {
		t.Fatalf("Expected successful list, got error: %v", errResult)
	}

	// Verify messageCount field exists
	resultList := result.([]interface{})
	for _, item := range resultList {
		itemMap := item.(map[string]interface{})
		if _, ok := itemMap["messageCount"]; !ok {
			t.Error("Missing messageCount field")
		}
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(ctx, "count_test")
}

// Test MDB002_5A_T9: Test list requires admin token (placeholder - auth not fully implemented)
func TestMDB002_5A_T9_ListRequiresAdminToken(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)

	// For now, just verify the method works
	// TODO: Add proper auth middleware checking in Phase 7
	result, errResult := makeDirectRPCCall(t, handler, "ns.list")
	if errResult != nil {
		t.Fatalf("Expected successful list: %v", errResult)
	}

	// Verify it's an array
	if _, ok := result.([]interface{}); !ok {
		t.Error("Expected array result from ns.list")
	}
}

// Test MDB002_5A_T10: Test info returns namespace stats
func TestMDB002_5A_T10_InfoReturnsNamespaceStats(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace with description
	opts := map[string]interface{}{
		"description": "Test namespace for stats",
	}
	_, errResult := makeDirectRPCCall(t, handler, "ns.create", "stats_test", opts)
	if errResult != nil {
		t.Fatalf("Failed to create namespace: %v", errResult)
	}

	// Get namespace info
	result, errResult := makeDirectRPCCall(t, handler, "ns.info", "stats_test")
	if errResult != nil {
		t.Fatalf("Expected successful info, got error: %v", errResult)
	}

	// Verify result structure
	resultMap := result.(map[string]interface{})

	if resultMap["namespace"] != "stats_test" {
		t.Errorf("Expected namespace 'stats_test', got '%v'", resultMap["namespace"])
	}

	if resultMap["description"] != "Test namespace for stats" {
		t.Errorf("Expected description 'Test namespace for stats', got '%v'", resultMap["description"])
	}

	// Verify required fields
	requiredFields := []string{"createdAt", "messageCount", "streamCount"}
	for _, field := range requiredFields {
		if _, ok := resultMap[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify createdAt is a string
	if _, ok := resultMap["createdAt"].(string); !ok {
		t.Error("createdAt should be a string")
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(ctx, "stats_test")
}

// Additional test: Create namespace with description and metadata
func TestMDB002_5A_CreateNamespaceWithOptions(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	ctx := context.Background()

	// Create namespace with options
	opts := map[string]interface{}{
		"description": "Production tenant A",
		"metadata": map[string]interface{}{
			"plan": "enterprise",
		},
	}

	result, errResult := makeDirectRPCCall(t, handler, "ns.create", "tenant_with_opts", opts)
	if errResult != nil {
		t.Fatalf("Expected success, got error: %v", errResult)
	}

	// Verify token returned
	resultMap := result.(map[string]interface{})
	if _, ok := resultMap["token"].(string); !ok {
		t.Error("Expected token in result")
	}

	// Verify description is stored
	ns, err := env.Store.GetNamespace(ctx, "tenant_with_opts")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.Description != "Production tenant A" {
		t.Errorf("Expected description 'Production tenant A', got '%s'", ns.Description)
	}

	// Cleanup
	_ = env.Store.DeleteNamespace(ctx, "tenant_with_opts")
}

// Additional test: Info for non-existent namespace
func TestMDB002_5A_InfoNonexistentNamespace(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	handler := api.NewRPCHandler("1.0.0", env.Store, nil)

	// Get info for non-existent namespace
	_, errResult := makeDirectRPCCall(t, handler, "ns.info", "nonexistent")
	if errResult == nil {
		t.Fatal("Expected error for non-existent namespace")
	}

	// Verify error code
	if errResult["code"] != "NAMESPACE_NOT_FOUND" {
		t.Errorf("Expected error code 'NAMESPACE_NOT_FOUND', got '%v'", errResult["code"])
	}
}
