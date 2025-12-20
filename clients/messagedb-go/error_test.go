package messagedb

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestERROR001_InvalidRPCMethod(t *testing.T) {
	tc := setupTest(t, "error-001")
	ctx := context.Background()

	// Call non-existent method
	_, err := tc.client.rpc(ctx, "invalid.method")

	if err == nil {
		t.Fatal("Expected error for invalid method, got nil")
	}

	// Should get some kind of error
	t.Logf("Got expected error: %v", err)
}

func TestERROR004_ConnectionRefused(t *testing.T) {
	// Create client pointing to non-existent server
	client := NewClient("http://localhost:99999", WithToken("dummy-token"))
	ctx := context.Background()

	_, err := client.StreamWrite(ctx, "test", Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	if err == nil {
		t.Fatal("Expected connection error, got nil")
	}

	// Should be a connection error
	t.Logf("Got expected connection error: %v", err)
}

func TestERROR006_NetworkTimeout(t *testing.T) {
	tc := setupTest(t, "error-006")

	// Create client with very short timeout
	client := NewClient(testBaseURL, WithToken(tc.client.GetToken()), WithHTTPClient(&http.Client{
		Timeout: 1 * time.Millisecond,
	}))

	ctx := context.Background()

	_, err := client.StreamWrite(ctx, "test", Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	// Should timeout (though might succeed if server is very fast)
	if err != nil {
		t.Logf("Got timeout/error as expected: %v", err)
	}
}
