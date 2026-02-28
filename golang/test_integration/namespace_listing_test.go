package integration

import (
	"context"
	"testing"

	"github.com/eventodb/eventodb/internal/api"
	"github.com/eventodb/eventodb/internal/store"
)

// writeMsg is a helper to write a message directly to the store
func writeMsg(t *testing.T, st store.Store, namespace, streamName, msgType string) {
	t.Helper()
	ctx := context.Background()
	_, err := st.WriteMessage(ctx, namespace, streamName, &store.Message{
		StreamName: streamName,
		Type:       msgType,
		Data:       map[string]interface{}{"test": true},
	})
	if err != nil {
		t.Fatalf("Failed to write message to %s: %v", streamName, err)
	}
}

// TestNsStreams_EmptyNamespace verifies empty array for empty namespace
func TestNsStreams_EmptyNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected array, got %T", result)
	}
	if len(arr) != 0 {
		t.Errorf("Expected empty array, got %d items", len(arr))
	}
}

// TestNsStreams_ReturnsStreamsAfterWrites verifies streams appear after writing
func TestNsStreams_ReturnsStreamsAfterWrites(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-2", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "order-1", "Created")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected array, got %T", result)
	}
	if len(arr) != 3 {
		t.Errorf("Expected 3 streams, got %d", len(arr))
	}

	// Verify structure
	first := arr[0].(map[string]interface{})
	if _, ok := first["stream"]; !ok {
		t.Error("Missing 'stream' field")
	}
	if _, ok := first["version"]; !ok {
		t.Error("Missing 'version' field")
	}
	if _, ok := first["lastActivity"]; !ok {
		t.Error("Missing 'lastActivity' field")
	}
}

// TestNsStreams_SortedLexicographically verifies alphabetical order
func TestNsStreams_SortedLexicographically(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "order-1", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "user-1", "Created")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	if len(arr) != 3 {
		t.Fatalf("Expected 3 streams, got %d", len(arr))
	}

	names := []string{
		arr[0].(map[string]interface{})["stream"].(string),
		arr[1].(map[string]interface{})["stream"].(string),
		arr[2].(map[string]interface{})["stream"].(string),
	}

	if names[0] != "account-1" || names[1] != "order-1" || names[2] != "user-1" {
		t.Errorf("Expected sorted order [account-1, order-1, user-1], got: %v", names)
	}
}

// TestNsStreams_PrefixFilter verifies prefix filtering
func TestNsStreams_PrefixFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-2", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "order-1", "Created")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams",
		map[string]interface{}{"prefix": "account"})
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	if len(arr) != 2 {
		t.Errorf("Expected 2 streams with prefix 'account', got %d", len(arr))
	}
	for _, item := range arr {
		s := item.(map[string]interface{})["stream"].(string)
		if len(s) < 7 || s[:7] != "account" {
			t.Errorf("Stream '%s' doesn't match prefix 'account'", s)
		}
	}
}

// TestNsStreams_CursorPagination verifies cursor-based pagination (no duplicates, no gaps)
func TestNsStreams_CursorPagination(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "stream-a", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "stream-b", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "stream-c", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "stream-d", "Created")

	// First page: limit 2
	result1, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams",
		map[string]interface{}{"limit": float64(2)})
	if err != nil {
		t.Fatalf("First page failed: %v", err)
	}
	arr1 := result1.([]interface{})
	if len(arr1) != 2 {
		t.Fatalf("Expected 2 on first page, got %d", len(arr1))
	}

	lastName := arr1[1].(map[string]interface{})["stream"].(string)

	// Second page: cursor after last of first page
	result2, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams",
		map[string]interface{}{"limit": float64(2), "cursor": lastName})
	if err != nil {
		t.Fatalf("Second page failed: %v", err)
	}
	arr2 := result2.([]interface{})
	if len(arr2) != 2 {
		t.Fatalf("Expected 2 on second page, got %d", len(arr2))
	}

	// No overlap
	for _, item := range arr2 {
		name := item.(map[string]interface{})["stream"].(string)
		if name <= lastName {
			t.Errorf("Pagination overlap: '%s' should be > '%s'", name, lastName)
		}
	}

	// All 4 covered
	allNames := map[string]bool{}
	for _, item := range arr1 {
		allNames[item.(map[string]interface{})["stream"].(string)] = true
	}
	for _, item := range arr2 {
		allNames[item.(map[string]interface{})["stream"].(string)] = true
	}
	if len(allNames) != 4 {
		t.Errorf("Expected 4 unique streams across pages, got %d: %v", len(allNames), allNames)
	}
}

// TestNsStreams_VersionAndLastActivity verifies version and lastActivity accuracy
func TestNsStreams_VersionAndLastActivity(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "A")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "B")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "C")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.streams")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	if len(arr) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(arr))
	}

	item := arr[0].(map[string]interface{})
	version, ok := item["version"].(float64)
	if !ok {
		t.Fatalf("version is not a number: %T %v", item["version"], item["version"])
	}
	if int64(version) != 2 {
		t.Errorf("Expected version 2 (0-based last position), got %v", version)
	}

	lastActivity, ok := item["lastActivity"].(string)
	if !ok || lastActivity == "" {
		t.Error("Expected non-empty lastActivity string")
	}
}

// TestNsStreams_NamespaceScoped verifies isolation between namespaces using store directly
func TestNsStreams_NamespaceScoped(t *testing.T) {
	ctx := context.Background()
	envA := SetupTestEnv(t)
	defer envA.Cleanup()
	envB := SetupTestEnv(t)
	defer envB.Cleanup()

	writeMsg(t, envA.Store, envA.Namespace, "account-1", "Created")

	streamsA, err := envA.Store.ListStreams(ctx, envA.Namespace, nil)
	if err != nil {
		t.Fatalf("ns-A ListStreams failed: %v", err)
	}
	if len(streamsA) != 1 {
		t.Errorf("ns-A expected 1 stream, got %d", len(streamsA))
	}

	streamsB, err := envB.Store.ListStreams(ctx, envB.Namespace, nil)
	if err != nil {
		t.Fatalf("ns-B ListStreams failed: %v", err)
	}
	if len(streamsB) != 0 {
		t.Errorf("ns-B expected 0 streams, got %d", len(streamsB))
	}
}

// TestNsStreams_RequiresAuth verifies the method requires auth when not in test mode
func TestNsStreams_RequiresAuth(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create handler without test mode - auth is enforced
	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	_, errResult := makeDirectRPCCall(t, handler, "ns.streams")
	if errResult == nil {
		t.Fatal("Expected error, got success")
	}
	if errResult["code"] != "AUTH_REQUIRED" {
		t.Errorf("Expected AUTH_REQUIRED, got: %v", errResult["code"])
	}
}

// TestNsCategories_EmptyNamespace verifies empty array for empty namespace
func TestNsCategories_EmptyNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.categories")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected array, got %T", result)
	}
	if len(arr) != 0 {
		t.Errorf("Expected empty array, got %d items", len(arr))
	}
}

// TestNsCategories_DerivesCategories verifies category extraction from stream names
func TestNsCategories_DerivesCategories(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-2", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "order-1", "Created")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.categories")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	if len(arr) != 2 {
		t.Fatalf("Expected 2 categories, got %d", len(arr))
	}

	c0 := arr[0].(map[string]interface{})
	c1 := arr[1].(map[string]interface{})

	if c0["category"] != "account" {
		t.Errorf("Expected first category 'account', got '%v'", c0["category"])
	}
	if c1["category"] != "order" {
		t.Errorf("Expected second category 'order', got '%v'", c1["category"])
	}
}

// TestNsCategories_CountsAreAccurate verifies streamCount and messageCount
func TestNsCategories_CountsAreAccurate(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "A")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "B")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-2", "A")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "order-1", "A")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.categories")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	if len(arr) != 2 {
		t.Fatalf("Expected 2 categories, got %d", len(arr))
	}

	catMap := map[string]map[string]interface{}{}
	for _, item := range arr {
		m := item.(map[string]interface{})
		catMap[m["category"].(string)] = m
	}

	acc := catMap["account"]
	if acc["streamCount"].(float64) != 2 {
		t.Errorf("account streamCount: expected 2, got %v", acc["streamCount"])
	}
	if acc["messageCount"].(float64) != 3 {
		t.Errorf("account messageCount: expected 3, got %v", acc["messageCount"])
	}

	ord := catMap["order"]
	if ord["streamCount"].(float64) != 1 {
		t.Errorf("order streamCount: expected 1, got %v", ord["streamCount"])
	}
	if ord["messageCount"].(float64) != 1 {
		t.Errorf("order messageCount: expected 1, got %v", ord["messageCount"])
	}
}

// TestNsCategories_StreamWithNoDash verifies bare stream names become their own category
func TestNsCategories_StreamWithNoDash(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account", "Created")
	writeMsg(t, ts.Env.Store, ts.Env.Namespace, "account-1", "Created")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.categories")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	arr := result.([]interface{})
	// Both "account" and "account-1" share the category "account"
	if len(arr) != 1 {
		t.Fatalf("Expected 1 category (both map to 'account'), got %d", len(arr))
	}

	cat := arr[0].(map[string]interface{})
	if cat["category"] != "account" {
		t.Errorf("Expected category 'account', got '%v'", cat["category"])
	}
	if cat["streamCount"].(float64) != 2 {
		t.Errorf("Expected streamCount 2, got %v", cat["streamCount"])
	}
	if cat["messageCount"].(float64) != 2 {
		t.Errorf("Expected messageCount 2, got %v", cat["messageCount"])
	}
}

// TestNsCategories_NamespaceScoped verifies isolation between namespaces using store directly
func TestNsCategories_NamespaceScoped(t *testing.T) {
	ctx := context.Background()
	envA := SetupTestEnv(t)
	defer envA.Cleanup()
	envB := SetupTestEnv(t)
	defer envB.Cleanup()

	writeMsg(t, envA.Store, envA.Namespace, "account-1", "Created")

	catsA, err := envA.Store.ListCategories(ctx, envA.Namespace)
	if err != nil {
		t.Fatalf("ns-A ListCategories failed: %v", err)
	}
	if len(catsA) != 1 {
		t.Errorf("ns-A expected 1 category, got %d", len(catsA))
	}

	catsB, err := envB.Store.ListCategories(ctx, envB.Namespace)
	if err != nil {
		t.Fatalf("ns-B ListCategories failed: %v", err)
	}
	if len(catsB) != 0 {
		t.Errorf("ns-B expected 0 categories, got %d", len(catsB))
	}
}

// TestNsCategories_RequiresAuth verifies the method requires auth when not in test mode
func TestNsCategories_RequiresAuth(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create handler without test mode - auth is enforced
	handler := api.NewRPCHandler("1.0.0", env.Store, nil)
	_, errResult := makeDirectRPCCall(t, handler, "ns.categories")
	if errResult == nil {
		t.Fatal("Expected error, got success")
	}
	if errResult["code"] != "AUTH_REQUIRED" {
		t.Errorf("Expected AUTH_REQUIRED, got: %v", errResult["code"])
	}
}
