package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestENCODING001_UTF8TextInData validates UTF-8 text in data field
func TestENCODING001_UTF8TextInData(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"text": "Hello 世界 🌍 émojis",
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	assert.Equal(t, "Hello 世界 🌍 émojis", data["text"])
}

// TestENCODING002_UnicodeInMetadata validates Unicode in metadata field
func TestENCODING002_UnicodeInMetadata(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
		"metadata": map[string]interface{}{
			"description": "Test 测试 🎉",
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	metadata := readMsg[5].(map[string]interface{})

	assert.Equal(t, "Test 测试 🎉", metadata["description"])
}

// TestENCODING003_SpecialCharactersInStreamName validates special characters in stream name
func TestENCODING003_SpecialCharactersInStreamName(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := "test-stream_123.abc"

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	// Should succeed if server allows, or clear error if not
	if err != nil {
		assert.Contains(t, err.Error(), "INVALID")
	} else {
		// If it succeeded, verify we can read it back
		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
		require.NoError(t, err)
		msgArray := result.([]interface{})
		assert.Len(t, msgArray, 1)
	}
}

// TestENCODING004_EmptyStringValues validates empty string values
func TestENCODING004_EmptyStringValues(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"emptyString": "",
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	// Should be empty string, not null
	assert.Equal(t, "", data["emptyString"])
	assert.NotNil(t, data["emptyString"])
}

// TestENCODING005_BooleanValues validates boolean values
func TestENCODING005_BooleanValues(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"isTrue":  true,
			"isFalse": false,
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	// Should be actual booleans, not strings
	assert.Equal(t, true, data["isTrue"])
	assert.Equal(t, false, data["isFalse"])
	assert.IsType(t, true, data["isTrue"])
}

// TestENCODING006_NullValues validates null values
func TestENCODING006_NullValues(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"nullValue": nil,
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	// Should be null
	assert.Nil(t, data["nullValue"])
}

// TestENCODING007_NumericValues validates numeric values
func TestENCODING007_NumericValues(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"integer":  42.0,
			"float":    3.14159,
			"negative": -100.0,
			"zero":     0.0,
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	assert.Equal(t, 42.0, data["integer"])
	assert.Equal(t, 3.14159, data["float"])
	assert.Equal(t, -100.0, data["negative"])
	assert.Equal(t, 0.0, data["zero"])
}

// TestENCODING008_NestedObjects validates nested objects
func TestENCODING008_NestedObjects(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": map[string]interface{}{
						"value": "deep",
					},
				},
			},
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	level1 := data["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	level3 := level2["level3"].(map[string]interface{})

	assert.Equal(t, "deep", level3["value"])
}

// TestENCODING009_ArraysInData validates arrays in data
func TestENCODING009_ArraysInData(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{
			"items": []interface{}{
				1.0,
				"two",
				map[string]interface{}{"three": 3.0},
				nil,
				true,
			},
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	// Read back
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	readMsg := msgArray[0].([]interface{})
	data := readMsg[4].(map[string]interface{})

	items := data["items"].([]interface{})
	assert.Len(t, items, 5)
	assert.Equal(t, 1.0, items[0])
	assert.Equal(t, "two", items[1])
	assert.Equal(t, 3.0, items[2].(map[string]interface{})["three"])
	assert.Nil(t, items[3])
	assert.Equal(t, true, items[4])
}

// TestENCODING010_LargeMessagePayload validates large message payload
func TestENCODING010_LargeMessagePayload(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Create a reasonably large data object (not too large to avoid test timeouts)
	largeData := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeData[string(rune('a'+i%26))+"-"+string(rune('0'+i%10))] = "This is some test data to make the payload larger"
	}

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": largeData,
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	// Should succeed if under server limit
	if err != nil {
		assert.Contains(t, err.Error(), "size")
	} else {
		// Verify we can read it back
		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream)
		require.NoError(t, err)
		msgArray := result.([]interface{})
		assert.Len(t, msgArray, 1)
	}
}
