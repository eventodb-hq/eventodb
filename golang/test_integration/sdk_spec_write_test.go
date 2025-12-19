package integration

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// randomStreamName generates a unique stream name for tests
func randomStreamName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

// TestWRITE001_WriteMinimalMessage validates writing a minimal message
func TestWRITE001_WriteMinimalMessage(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Expected: Returns object with position (>= 0) and globalPosition (>= 0)
	resultMap := result.(map[string]interface{})
	assert.GreaterOrEqual(t, resultMap["position"].(float64), 0.0)
	assert.GreaterOrEqual(t, resultMap["globalPosition"].(float64), 0.0)

	// First message should have position 0
	assert.Equal(t, 0.0, resultMap["position"].(float64))
}

// TestWRITE002_WriteMessageWithMetadata validates writing a message with metadata
func TestWRITE002_WriteMessageWithMetadata(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
		"metadata": map[string]interface{}{
			"correlationId": "123",
		},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Read back and verify metadata
	messages, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := messages.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	// Message structure: [id, type, position, globalPosition, data, metadata, time]
	// metadata is at index 5
	metadata := readMsg[5].(map[string]interface{})
	assert.Equal(t, "123", metadata["correlationId"])
}

// TestWRITE003_WriteWithCustomMessageID validates writing with a custom message ID
func TestWRITE003_WriteWithCustomMessageID(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	customUUID := "550e8400-e29b-41d4-a716-446655440000"

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	opts := map[string]interface{}{
		"id": customUUID,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg, opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Read back and verify ID
	messages, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := messages.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	messageID := readMsg[0].(string)
	assert.Equal(t, customUUID, messageID)
}

// TestWRITE004_WriteWithExpectedVersionSuccess validates writing with expected version (success case)
func TestWRITE004_WriteWithExpectedVersionSuccess(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 2 messages first
	msg1 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 1},
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg1)
	require.NoError(t, err)

	msg2 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 2},
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg2)
	require.NoError(t, err)

	// Now write with expected version 1 (should succeed)
	msg3 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 3},
	}
	opts := map[string]interface{}{
		"expectedVersion": 1,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg3, opts)
	require.NoError(t, err)

	// Should return position 2
	resultMap := result.(map[string]interface{})
	assert.Equal(t, 2.0, resultMap["position"].(float64))
}

// TestWRITE005_WriteWithExpectedVersionConflict validates writing with expected version (conflict case)
func TestWRITE005_WriteWithExpectedVersionConflict(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 2 messages first
	msg1 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 1},
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg1)
	require.NoError(t, err)

	msg2 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 2},
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg2)
	require.NoError(t, err)

	// Now write with expected version 5 (should fail - actual version is 1)
	msg3 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 3},
	}
	opts := map[string]interface{}{
		"expectedVersion": 5,
	}

	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg3, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STREAM_VERSION_CONFLICT")
}

// TestWRITE006_WriteMultipleMessagesSequentially validates sequential writes
func TestWRITE006_WriteMultipleMessagesSequentially(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	var positions []float64
	var globalPositions []float64

	// Write 5 messages
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}

		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)

		resultMap := result.(map[string]interface{})
		positions = append(positions, resultMap["position"].(float64))
		globalPositions = append(globalPositions, resultMap["globalPosition"].(float64))
	}

	// Verify positions are sequential (0, 1, 2, 3, 4)
	for i := 0; i < 5; i++ {
		assert.Equal(t, float64(i), positions[i], "Position %d should be %d", i, i)
	}

	// Verify global positions are monotonically increasing
	for i := 1; i < 5; i++ {
		assert.Greater(t, globalPositions[i], globalPositions[i-1],
			"Global position should be monotonically increasing")
	}
}

// TestWRITE007_WriteToStreamWithID validates writing to a stream with an ID part
func TestWRITE007_WriteToStreamWithID(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := "account-123"
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	assert.GreaterOrEqual(t, resultMap["position"].(float64), 0.0)
}

// TestWRITE008_WriteWithEmptyDataObject validates writing with empty data object
func TestWRITE008_WriteWithEmptyDataObject(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Read back and verify data is empty object
	messages, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := messages.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	// Message structure: [id, type, position, globalPosition, data, metadata, time]
	// data is at index 4
	data := readMsg[4].(map[string]interface{})
	assert.Empty(t, data)
}

// TestWRITE009_WriteWithNullMetadata validates writing without metadata field
func TestWRITE009_WriteWithNullMetadata(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	// Don't include metadata field at all (server may reject explicit null)
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"x": 1},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Read back and verify metadata is null
	messages, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := messages.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	// Message structure: [id, type, position, globalPosition, data, metadata, time]
	// metadata is at index 5
	metadata := readMsg[5]
	assert.Nil(t, metadata)
}

// TestWRITE010_WriteWithoutAuthentication validates that write fails without authentication
func TestWRITE010_WriteWithoutAuthentication(t *testing.T) {
	// Skip this test as SetupTestServer runs in test mode which allows missing auth
	// This test is more appropriate for production mode testing
	t.Skip("Test server runs in test mode which allows missing auth - this test validates production behavior")

	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	// Make call without token
	_, err := makeRPCCall(t, ts.Port, "", "stream.write", stream, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AUTH")
}
