package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSYS001_GetServerVersion validates getting server version
func TestSYS001_GetServerVersion(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Call version endpoint directly (not RPC)
	resp, err := http.Get(ts.URL() + "/version")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Should contain version in JSON format
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "version")
	assert.Contains(t, bodyStr, "1.4.0")
}

// TestSYS002_GetServerHealth validates getting server health
func TestSYS002_GetServerHealth(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Call health endpoint directly
	resp, err := http.Get(ts.URL() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Should contain status: ok
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "status")
	assert.Contains(t, bodyStr, "ok")
}
