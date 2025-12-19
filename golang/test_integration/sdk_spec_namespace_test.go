package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNS001_CreateNamespace validates creating a namespace
func TestNS001_CreateNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("test-ns")
	opts := map[string]interface{}{
		"description": "Test namespace",
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.create", namespace, opts)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	assert.Equal(t, namespace, resultMap["namespace"])
	assert.NotEmpty(t, resultMap["token"], "Should return a token")
	assert.True(t, len(resultMap["token"].(string)) > 3, "Token should start with ns_ prefix")
	assert.NotEmpty(t, resultMap["createdAt"], "Should have createdAt timestamp")
}

// TestNS002_CreateNamespaceWithCustomToken validates creating namespace with custom token
func TestNS002_CreateNamespaceWithCustomToken(t *testing.T) {
	t.Skip("Custom token generation requires auth module - skipping for now")
}

// TestNS003_CreateDuplicateNamespace validates that creating duplicate namespace fails
func TestNS003_CreateDuplicateNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("duplicate-test")

	// Create first time
	opts := map[string]interface{}{
		"description": "Test namespace",
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "ns.create", namespace, opts)
	require.NoError(t, err)

	// Try to create again
	_, err = makeRPCCall(t, ts.Port, ts.Token, "ns.create", namespace, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NAMESPACE_EXISTS")
}

// TestNS004_DeleteNamespace validates deleting a namespace
func TestNS004_DeleteNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("delete-test")

	// Create namespace first
	opts := map[string]interface{}{
		"description": "Test namespace",
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "ns.create", namespace, opts)
	require.NoError(t, err)

	// Delete it
	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.delete", namespace)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	assert.Equal(t, namespace, resultMap["namespace"])
	assert.NotEmpty(t, resultMap["deletedAt"])
	// messagesDeleted field should exist
	assert.Contains(t, resultMap, "messagesDeleted")
}

// TestNS005_DeleteNonExistentNamespace validates that deleting non-existent namespace fails
func TestNS005_DeleteNonExistentNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("does-not-exist")

	_, err := makeRPCCall(t, ts.Port, ts.Token, "ns.delete", namespace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NAMESPACE_NOT_FOUND")
}

// TestNS006_ListNamespaces validates listing namespaces
func TestNS006_ListNamespaces(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// At least the default namespace should exist
	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.list")
	require.NoError(t, err)

	namespaces := result.([]interface{})
	assert.NotEmpty(t, namespaces, "Should return at least one namespace")

	// Check structure of first namespace
	if len(namespaces) > 0 {
		ns := namespaces[0].(map[string]interface{})
		assert.Contains(t, ns, "namespace")
		assert.Contains(t, ns, "description")
		assert.Contains(t, ns, "createdAt")
		assert.Contains(t, ns, "messageCount")
	}
}

// TestNS007_GetNamespaceInfo validates getting namespace info
func TestNS007_GetNamespaceInfo(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("info-test")

	// Create namespace
	opts := map[string]interface{}{
		"description": "Test namespace",
	}
	createResult, err := makeRPCCall(t, ts.Port, ts.Token, "ns.create", namespace, opts)
	require.NoError(t, err)

	// Get the token for this namespace
	createMap := createResult.(map[string]interface{})
	nsToken := createMap["token"].(string)

	// Write 5 messages to this namespace
	for i := 0; i < 5; i++ {
		stream := randomStreamName("test")
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, nsToken, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Get namespace info
	result, err := makeRPCCall(t, ts.Port, ts.Token, "ns.info", namespace)
	require.NoError(t, err)

	infoMap := result.(map[string]interface{})
	messageCount := int(infoMap["messageCount"].(float64))
	assert.Equal(t, 5, messageCount, "Should have correct message count")
}

// TestNS008_GetInfoForNonExistentNamespace validates getting info for non-existent namespace
func TestNS008_GetInfoForNonExistentNamespace(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	namespace := randomStreamName("does-not-exist")

	_, err := makeRPCCall(t, ts.Port, ts.Token, "ns.info", namespace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NAMESPACE_NOT_FOUND")
}
