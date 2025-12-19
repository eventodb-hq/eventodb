package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestREAD001_ReadFromEmptyStream validates reading from a non-existent stream
func TestREAD001_ReadFromEmptyStream(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("nonexistent")

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	// Expected: Returns empty array
	msgArray := result.([]interface{})
	assert.Empty(t, msgArray)
}

// TestREAD002_ReadSingleMessage validates reading a single message
func TestREAD002_ReadSingleMessage(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write one message
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read it back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 1)

	// Verify message structure: [id, type, position, globalPosition, data, metadata, time]
	readMsg := msgArray[0].([]interface{})
	require.Len(t, readMsg, 7, "Message should have 7 fields")

	// Verify fields exist and have correct types
	assert.IsType(t, "", readMsg[0], "id should be string")
	assert.IsType(t, "", readMsg[1], "type should be string")
	assert.IsType(t, float64(0), readMsg[2], "position should be number")
	assert.IsType(t, float64(0), readMsg[3], "globalPosition should be number")
	assert.NotNil(t, readMsg[4], "data should exist")
	// metadata can be nil or object
	assert.IsType(t, "", readMsg[6], "time should be string")
}

// TestREAD003_ReadMultipleMessages validates reading multiple messages
func TestREAD003_ReadMultipleMessages(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 5 messages
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read them back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 5)

	// Verify positions are in order (0, 1, 2, 3, 4)
	for i := 0; i < 5; i++ {
		readMsg := msgArray[i].([]interface{})
		position := readMsg[2].(float64)
		assert.Equal(t, float64(i), position, "Position should be %d", i)
	}
}

// TestREAD004_ReadWithPositionFilter validates reading from a specific position
func TestREAD004_ReadWithPositionFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 10 messages
	for i := 0; i < 10; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read from position 5
	opts := map[string]interface{}{
		"position": 5,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 5, "Should return messages at positions 5-9")

	// Verify positions
	for i, msg := range msgArray {
		readMsg := msg.([]interface{})
		position := readMsg[2].(float64)
		assert.Equal(t, float64(5+i), position)
	}
}

// TestREAD005_ReadWithGlobalPositionFilter validates reading from a specific global position
func TestREAD005_ReadWithGlobalPositionFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	var globalPositions []int64

	// Write 4 messages and capture their global positions
	for i := 0; i < 4; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)

		resultMap := result.(map[string]interface{})
		globalPositions = append(globalPositions, int64(resultMap["globalPosition"].(float64)))
	}

	// Read from global position of 3rd message (index 2)
	opts := map[string]interface{}{
		"globalPosition": globalPositions[2],
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 2, "Should return messages from globalPosition of 3rd and 4th message")

	// Verify global positions
	readMsg1 := msgArray[0].([]interface{})
	gp1 := int64(readMsg1[3].(float64))
	assert.Equal(t, globalPositions[2], gp1)

	readMsg2 := msgArray[1].([]interface{})
	gp2 := int64(readMsg2[3].(float64))
	assert.Equal(t, globalPositions[3], gp2)
}

// TestREAD006_ReadWithBatchSizeLimit validates reading with batch size limit
func TestREAD006_ReadWithBatchSizeLimit(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 100 messages
	for i := 0; i < 100; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read with batch size 10
	opts := map[string]interface{}{
		"batchSize": 10,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	assert.Len(t, msgArray, 10, "Should return exactly 10 messages")

	// Verify positions 0-9
	for i := 0; i < 10; i++ {
		readMsg := msgArray[i].([]interface{})
		position := readMsg[2].(float64)
		assert.Equal(t, float64(i), position)
	}
}

// TestREAD007_ReadWithBatchSizeUnlimited validates reading with unlimited batch size
func TestREAD007_ReadWithBatchSizeUnlimited(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 50 messages
	for i := 0; i < 50; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read with batch size -1 (unlimited)
	opts := map[string]interface{}{
		"batchSize": -1,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	assert.Len(t, msgArray, 50, "Should return all 50 messages")
}

// TestREAD008_ReadMessageDataIntegrity validates data field integrity
func TestREAD008_ReadMessageDataIntegrity(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write message with complex data
	complexData := map[string]interface{}{
		"nested": map[string]interface{}{
			"array": []interface{}{1.0, 2.0, 3.0},
			"bool":  true,
			"null":  nil,
		},
	}

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": complexData,
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	// Verify nested structure
	nested := data["nested"].(map[string]interface{})
	array := nested["array"].([]interface{})
	assert.Equal(t, []interface{}{1.0, 2.0, 3.0}, array)
	assert.Equal(t, true, nested["bool"])
	assert.Nil(t, nested["null"])
}

// TestREAD009_ReadMessageMetadataIntegrity validates metadata field integrity
func TestREAD009_ReadMessageMetadataIntegrity(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write message with metadata
	metadata := map[string]interface{}{
		"correlationId": "123",
		"userId":        "user-456",
	}

	msg := map[string]interface{}{
		"type":     "TestEvent",
		"data":     map[string]interface{}{"foo": "bar"},
		"metadata": metadata,
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	readMetadata := readMsg[5].(map[string]interface{})

	// Verify exact match
	assert.Equal(t, "123", readMetadata["correlationId"])
	assert.Equal(t, "user-456", readMetadata["userId"])
}

// TestREAD010_ReadMessageTimestampFormat validates timestamp format
func TestREAD010_ReadMessageTimestampFormat(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write a message
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	require.Len(t, msgArray, 1)

	readMsg := msgArray[0].([]interface{})
	timestamp := readMsg[6].(string)

	// Verify it's a valid ISO 8601 timestamp
	parsedTime, err := time.Parse(time.RFC3339Nano, timestamp)
	require.NoError(t, err, "Timestamp should be valid ISO 8601 format")

	// Verify it ends with Z (UTC)
	assert.Contains(t, timestamp, "Z", "Timestamp should be in UTC")

	// Verify it's a recent timestamp (within last minute)
	assert.WithinDuration(t, time.Now(), parsedTime, time.Minute)
}

// LAST tests

// TestLAST001_LastMessageFromNonEmptyStream validates getting last message
func TestLAST001_LastMessageFromNonEmptyStream(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 5 messages
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Get last message
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.last", stream)
	require.NoError(t, err)

	// Should be an array with 7 elements (message structure)
	lastMsg := result.([]interface{})
	require.Len(t, lastMsg, 7)

	// Should be at position 4
	position := lastMsg[2].(float64)
	assert.Equal(t, 4.0, position)
}

// TestLAST002_LastMessageFromEmptyStream validates getting last message from empty stream
func TestLAST002_LastMessageFromEmptyStream(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("nonexistent")

	// Get last message from non-existent stream
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.last", stream)
	require.NoError(t, err)

	// Should be null
	assert.Nil(t, result)
}

// TestLAST003_LastMessageFilteredByType validates filtering last message by type
func TestLAST003_LastMessageFilteredByType(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write messages: [TypeA, TypeB, TypeA, TypeB, TypeA]
	types := []string{"TypeA", "TypeB", "TypeA", "TypeB", "TypeA"}
	for i, msgType := range types {
		msg := map[string]interface{}{
			"type": msgType,
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Get last TypeB message
	opts := map[string]interface{}{
		"type": "TypeB",
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.last", stream, opts)
	require.NoError(t, err)

	lastMsg := result.([]interface{})
	require.NotNil(t, lastMsg)

	// Should be at position 3 (last TypeB)
	position := lastMsg[2].(float64)
	assert.Equal(t, 3.0, position)

	// Verify type
	msgType := lastMsg[1].(string)
	assert.Equal(t, "TypeB", msgType)
}

// TestLAST004_LastMessageTypeFilterNoMatch validates filtering with no matches
func TestLAST004_LastMessageTypeFilterNoMatch(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write only TypeA messages
	for i := 0; i < 3; i++ {
		msg := map[string]interface{}{
			"type": "TypeA",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Get last TypeB message (doesn't exist)
	opts := map[string]interface{}{
		"type": "TypeB",
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.last", stream, opts)
	require.NoError(t, err)

	// Should be null
	assert.Nil(t, result)
}
