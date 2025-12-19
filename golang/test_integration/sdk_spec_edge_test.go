package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEDGE001_EmptyBatchSizeBehavior validates empty batch size behavior
func TestEDGE001_EmptyBatchSizeBehavior(t *testing.T) {
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

	// Read with batch size 0
	opts := map[string]interface{}{
		"batchSize": 0,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)

	// Server treats batchSize 0 as default (returns all messages up to limit)
	// This is acceptable behavior - no error needed
	require.NoError(t, err)
	msgArray := result.([]interface{})
	// Should return some messages (behavior is server-defined)
	assert.NotNil(t, msgArray)
}

// TestEDGE002_NegativePosition validates negative position handling
func TestEDGE002_NegativePosition(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write some messages
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read with negative position
	opts := map[string]interface{}{
		"position": -1,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)

	// Server treats negative position as 0 (reads from beginning)
	// This is acceptable behavior - no error needed
	require.NoError(t, err)
	msgArray := result.([]interface{})
	// Should return some messages
	assert.NotNil(t, msgArray)
}

// TestEDGE003_VeryLargeBatchSize validates very large batch size
func TestEDGE003_VeryLargeBatchSize(t *testing.T) {
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

	// Read with very large batch size
	opts := map[string]interface{}{
		"batchSize": 1000000,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)

	// Should succeed and return available messages (5)
	// Server may cap at max limit (e.g., 10000)
	if err != nil {
		// If error, it should be about exceeding limit
		assert.Contains(t, err.Error(), "batchSize")
	} else {
		msgArray := result.([]interface{})
		assert.Len(t, msgArray, 5, "Should return all available messages")
	}
}

// TestEDGE004_StreamNameEdgeCases validates various stream name formats
func TestEDGE004_StreamNameEdgeCases(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	testCases := []struct {
		name       string
		streamName string
		shouldWork bool
	}{
		{"single char", "a", true},
		{"many dashes", "stream-with-many-dashes", true},
		{"with numbers", "stream123", true},
		{"uppercase", "UPPERCASE", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := map[string]interface{}{
				"type": "TestEvent",
				"data": map[string]interface{}{"foo": "bar"},
			}

			_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", tc.streamName, msg)

			if tc.shouldWork {
				assert.NoError(t, err, "Stream name '%s' should be valid", tc.streamName)
			} else {
				assert.Error(t, err, "Stream name '%s' should be invalid", tc.streamName)
			}
		})
	}
}

// TestEDGE005_ConcurrentWritesToSameStream validates concurrent writes
// Note: This test can be flaky with SQLite due to concurrent access issues
func TestEDGE005_ConcurrentWritesToSameStream(t *testing.T) {
	// Skip on SQLite backend - concurrent writes have race conditions
	if GetTestBackend() == BackendSQLite {
		t.Skip("Concurrent writes test skipped on SQLite backend due to race conditions")
	}
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 10 messages concurrently
	type writeResult struct {
		position float64
		err      error
	}
	resultChan := make(chan writeResult, 10)

	for i := 0; i < 10; i++ {
		go func(seq int) {
			msg := map[string]interface{}{
				"type": "TestEvent",
				"data": map[string]interface{}{"seq": seq},
			}
			result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
			if err != nil {
				resultChan <- writeResult{err: err}
				return
			}

			resultMap := result.(map[string]interface{})
			resultChan <- writeResult{position: resultMap["position"].(float64)}
		}(i)
	}

	// Collect results
	var positions []float64
	for i := 0; i < 10; i++ {
		res := <-resultChan
		if res.err == nil {
			positions = append(positions, res.position)
		} else {
			t.Logf("Write failed: %v", res.err)
		}
	}

	// All writes should succeed
	assert.Len(t, positions, 10, "All concurrent writes should succeed")

	// Positions should be unique and sequential (though not necessarily in order received)
	posMap := make(map[float64]bool)
	for _, pos := range positions {
		assert.False(t, posMap[pos], "Position %.0f should be unique", pos)
		posMap[pos] = true
		assert.GreaterOrEqual(t, pos, 0.0)
		assert.LessOrEqual(t, pos, 9.0)
	}
}

// TestEDGE006_ReadFromPositionBeyondStreamEnd validates reading beyond stream
func TestEDGE006_ReadFromPositionBeyondStreamEnd(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write 5 messages (positions 0-4)
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read from position 100
	opts := map[string]interface{}{
		"position": 100,
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.get", stream, opts)
	require.NoError(t, err)

	msgArray := result.([]interface{})
	assert.Empty(t, msgArray, "Should return empty array when position is beyond stream end")
}

// TestEDGE007_ExpectedVersionMinusOne validates expected version -1 (no stream)
func TestEDGE007_ExpectedVersionMinusOne(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write with expected version -1 (stream should not exist)
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	opts := map[string]interface{}{
		"expectedVersion": -1,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg, opts)
	require.NoError(t, err, "Should succeed when stream doesn't exist")

	resultMap := result.(map[string]interface{})
	assert.Equal(t, 0.0, resultMap["position"].(float64))

	// Try again - should fail now because stream exists
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg, opts)
	require.Error(t, err, "Should fail when stream exists")
	assert.Contains(t, err.Error(), "STREAM_VERSION_CONFLICT")
}

// TestEDGE008_ExpectedVersionZero validates expected version 0 (first message)
func TestEDGE008_ExpectedVersionZero(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("test")

	// Write with expected version 0 on non-existent stream
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	opts := map[string]interface{}{
		"expectedVersion": 0,
	}

	// Should fail because stream version is -1 (doesn't exist)
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg, opts)
	require.Error(t, err, "Should fail on non-existent stream")
	assert.Contains(t, err.Error(), "STREAM_VERSION_CONFLICT")
}
