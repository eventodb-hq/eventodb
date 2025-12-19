package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVERSION001_VersionOfNonExistentStream validates getting version of non-existent stream
func TestVERSION001_VersionOfNonExistentStream(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("nonexistent")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.version", stream)
	require.NoError(t, err)

	// Should return null for non-existent stream
	assert.Nil(t, result)
}

// TestVERSION002_VersionOfStreamWithMessages validates getting version of stream with messages
func TestVERSION002_VersionOfStreamWithMessages(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 3 messages (positions 0, 1, 2)
	for i := 0; i < 3; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Get version
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.version", stream)
	require.NoError(t, err)

	// Should return 2 (last position, 0-indexed)
	version := result.(float64)
	assert.Equal(t, 2.0, version)
}

// TestVERSION003_VersionAfterWrite validates version updates after write
func TestVERSION003_VersionAfterWrite(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write first message
	msg1 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 1},
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg1)
	require.NoError(t, err)

	// Check version (should be 0)
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.version", stream)
	require.NoError(t, err)
	version1 := result.(float64)
	assert.Equal(t, 0.0, version1)

	// Write another message
	msg2 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 2},
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg2)
	require.NoError(t, err)

	// Check version again (should be 1)
	result, err = makeRPCCall(t, ts.Port, ts.Token, "stream.version", stream)
	require.NoError(t, err)
	version2 := result.(float64)
	assert.Equal(t, 1.0, version2)
}
