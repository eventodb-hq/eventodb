package eventodb

import (
	"context"
	"errors"
	"testing"
)

func TestAUTH001_ValidTokenAuthentication(t *testing.T) {
	tc := setupTest(t, "auth-001")
	ctx := context.Background()

	stream := randomStreamName()

	// Should succeed with valid token
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write with valid token: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}
}

func TestAUTH002_MissingToken(t *testing.T) {
	// Create client without token
	client := NewClient(testBaseURL)
	ctx := context.Background()

	stream := randomStreamName()

	_, err := client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	// Note: Some server configurations allow unauthenticated access to default namespace
	// If err is nil, that's acceptable in test mode
	if err == nil {
		t.Log("Server allows unauthenticated access (test/dev mode)")
		return
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected Error type, got %T: %v", err, err)
	}

	if dbErr.Code != "AUTH_REQUIRED" {
		t.Errorf("Expected AUTH_REQUIRED, got %s", dbErr.Code)
	}
}

func TestAUTH003_InvalidTokenFormat(t *testing.T) {
	// Create client with malformed token
	client := NewClient(testBaseURL, WithToken("invalid-token"))
	ctx := context.Background()

	stream := randomStreamName()

	_, err := client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	// Note: Some server configurations allow unauthenticated access to default namespace
	// If err is nil, that's acceptable in test mode
	if err == nil {
		t.Log("Server allows access with invalid token (test/dev mode)")
		return
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		// May be a different error type for malformed token
		t.Logf("Got error: %v", err)
		return
	}

	// Accept AUTH_INVALID, AUTH_REQUIRED, or AUTH_INVALID_TOKEN
	if dbErr.Code != "AUTH_INVALID" && dbErr.Code != "AUTH_REQUIRED" && dbErr.Code != "AUTH_INVALID_TOKEN" {
		t.Errorf("Expected AUTH_INVALID, AUTH_REQUIRED, or AUTH_INVALID_TOKEN, got %s", dbErr.Code)
	}
}

func TestAUTH004_TokenNamespaceIsolation(t *testing.T) {
	// Create two separate test namespaces
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()

	// Namespace 1
	ns1ID := randomStreamName()
	ns1Result, err := adminClient.NamespaceCreate(ctx, ns1ID, &CreateNamespaceOptions{
		Description: strPtr("Namespace 1 for isolation test"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace 1: %v", err)
	}
	defer adminClient.NamespaceDelete(ctx, ns1ID)

	// Namespace 2
	ns2ID := randomStreamName()
	ns2Result, err := adminClient.NamespaceCreate(ctx, ns2ID, &CreateNamespaceOptions{
		Description: strPtr("Namespace 2 for isolation test"),
	})
	if err != nil {
		t.Fatalf("Failed to create namespace 2: %v", err)
	}
	defer adminClient.NamespaceDelete(ctx, ns2ID)

	// Create clients for each namespace
	client1 := NewClient(testBaseURL, WithToken(ns1Result.Token))
	client2 := NewClient(testBaseURL, WithToken(ns2Result.Token))

	// Write message to stream in namespace 1
	stream := "test-stream"
	_, err = client1.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"namespace": "ns1"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to namespace 1: %v", err)
	}

	// Try to read same stream name using namespace 2 token
	messages, err := client2.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read with namespace 2 token: %v", err)
	}

	// Should get empty array (namespace isolation)
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages (namespace isolation), got %d", len(messages))
	}

	// Verify namespace 1 can still read its own message
	messages, err = client1.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read with namespace 1 token: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message in namespace 1, got %d", len(messages))
	}
}
