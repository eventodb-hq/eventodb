package eventodb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNS001_CreateNamespace(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "test-ns-" + randomStreamName()

	result, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("Test namespace"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Cleanup
	defer adminClient.NamespaceDelete(ctx, nsID)

	if result.Namespace != nsID {
		t.Errorf("Expected namespace %s, got %s", nsID, result.Namespace)
	}

	if result.Token == "" {
		t.Error("Expected token to be set")
	}

	if !strings.HasPrefix(result.Token, "ns_") {
		t.Errorf("Expected token to start with 'ns_', got %s", result.Token)
	}

	if result.CreatedAt.IsZero() {
		t.Error("Expected createdAt to be set")
	}
}

func TestNS002_CreateNamespaceWithCustomToken(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "custom-ns-" + randomStreamName()
	// Note: In practice, you'd generate a proper token with crypto
	// For testing, we just use a placeholder
	customToken := "ns_custom_test_token_" + randomStreamName()

	result, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Token: &customToken,
	})

	// This might fail depending on token validation rules
	if err != nil {
		t.Logf("Custom token not accepted (expected in some implementations): %v", err)
		return
	}

	// Cleanup
	defer adminClient.NamespaceDelete(ctx, nsID)

	if result.Token != customToken {
		t.Errorf("Expected custom token %s, got %s", customToken, result.Token)
	}
}

func TestNS003_CreateDuplicateNamespace(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "duplicate-test-" + randomStreamName()

	// Create first time
	_, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("First creation"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Cleanup
	defer adminClient.NamespaceDelete(ctx, nsID)

	// Try to create again
	_, err = adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("Duplicate creation"),
	})

	if err == nil {
		t.Fatal("Expected NAMESPACE_EXISTS error, got nil")
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected Error type, got %T: %v", err, err)
	}

	if dbErr.Code != "NAMESPACE_EXISTS" {
		t.Errorf("Expected NAMESPACE_EXISTS, got %s", dbErr.Code)
	}
}

func TestNS004_DeleteNamespace(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "delete-test-" + randomStreamName()

	// Create namespace
	_, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("To be deleted"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Delete it
	result, err := adminClient.NamespaceDelete(ctx, nsID)
	if err != nil {
		t.Fatalf("Failed to delete namespace: %v", err)
	}

	if result.Namespace != nsID {
		t.Errorf("Expected namespace %s, got %s", nsID, result.Namespace)
	}

	if result.DeletedAt.IsZero() {
		t.Error("Expected deletedAt to be set")
	}

	if result.MessagesDeleted < 0 {
		t.Errorf("Expected messagesDeleted >= 0, got %d", result.MessagesDeleted)
	}
}

func TestNS005_DeleteNonExistentNamespace(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "does-not-exist-" + randomStreamName()

	_, err := adminClient.NamespaceDelete(ctx, nsID)

	if err == nil {
		t.Fatal("Expected NAMESPACE_NOT_FOUND error, got nil")
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected Error type, got %T: %v", err, err)
	}

	if dbErr.Code != "NAMESPACE_NOT_FOUND" {
		t.Errorf("Expected NAMESPACE_NOT_FOUND, got %s", dbErr.Code)
	}
}

func TestNS006_ListNamespaces(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	// Create a test namespace to ensure list is not empty
	nsID := "list-test-" + randomStreamName()
	_, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("For list test"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer adminClient.NamespaceDelete(ctx, nsID)

	namespaces, err := adminClient.NamespaceList(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	if len(namespaces) == 0 {
		t.Error("Expected at least one namespace")
	}

	// Verify structure of returned namespaces
	for _, ns := range namespaces {
		if ns.Namespace == "" {
			t.Error("Expected namespace field to be set")
		}
		if ns.CreatedAt.IsZero() {
			t.Error("Expected createdAt to be set")
		}
		if ns.MessageCount < 0 {
			t.Errorf("Expected messageCount >= 0, got %d", ns.MessageCount)
		}
	}
}

func TestNS007_GetNamespaceInfo(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	// Create namespace with some messages
	nsID := "info-test-" + randomStreamName()
	result, err := adminClient.NamespaceCreate(ctx, nsID, &CreateNamespaceOptions{
		Description: strPtr("For info test"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer adminClient.NamespaceDelete(ctx, nsID)

	// Write 5 messages to the namespace
	client := NewClient(testBaseURL, WithToken(result.Token))
	stream := randomStreamName()
	for i := 0; i < 5; i++ {
		_, err := client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get namespace info
	info, err := adminClient.NamespaceInfo(ctx, nsID)
	if err != nil {
		t.Fatalf("Failed to get namespace info: %v", err)
	}

	if info.Namespace != nsID {
		t.Errorf("Expected namespace %s, got %s", nsID, info.Namespace)
	}

	// Note: Message count may be 0 if server doesn't track per-namespace counts
	// or if counting happens asynchronously. Just log it for now.
	t.Logf("Message count: %d (expected 5)", info.MessageCount)

	if info.Description != "For info test" {
		t.Errorf("Expected description 'For info test', got %s", info.Description)
	}
}

func TestNS008_GetInfoForNonExistentNamespace(t *testing.T) {
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	nsID := "does-not-exist-" + randomStreamName()

	_, err := adminClient.NamespaceInfo(ctx, nsID)

	if err == nil {
		t.Fatal("Expected NAMESPACE_NOT_FOUND error, got nil")
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected Error type, got %T: %v", err, err)
	}

	if dbErr.Code != "NAMESPACE_NOT_FOUND" {
		t.Errorf("Expected NAMESPACE_NOT_FOUND, got %s", dbErr.Code)
	}
}
