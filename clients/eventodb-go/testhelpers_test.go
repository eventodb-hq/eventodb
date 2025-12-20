package eventodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

var (
	testBaseURL    = getEnv("EVENTODB_URL", "http://localhost:8080")
	testAdminToken = getEnv("EVENTODB_ADMIN_TOKEN", "")
)

type testContext struct {
	client      *Client
	namespaceID string
	t           *testing.T
}

func setupTest(t *testing.T, testName string) *testContext {
	t.Helper()

	// Create admin client
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))

	// Create test namespace
	namespaceID := fmt.Sprintf("test-%s-%d", testName, time.Now().UnixNano())

	ctx := context.Background()
	result, err := adminClient.NamespaceCreate(ctx, namespaceID, &CreateNamespaceOptions{
		Description: strPtr(fmt.Sprintf("Test namespace for %s", testName)),
	})
	if err != nil {
		t.Fatalf("Failed to create test namespace: %v", err)
	}

	// Create client with namespace token
	client := NewClient(testBaseURL, WithToken(result.Token))

	tc := &testContext{
		client:      client,
		namespaceID: namespaceID,
		t:           t,
	}

	// Register cleanup
	t.Cleanup(func() {
		tc.cleanup()
	})

	return tc
}

func (tc *testContext) cleanup() {
	// Delete test namespace
	adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
	ctx := context.Background()
	_, _ = adminClient.NamespaceDelete(ctx, tc.namespaceID)
}

func randomStreamName() string {
	return fmt.Sprintf("test%d", time.Now().UnixNano())
}

func randomCategoryName() string {
	return fmt.Sprintf("cat%d", time.Now().UnixNano())
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func strPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func intPtr(i int) *int {
	return &i
}
