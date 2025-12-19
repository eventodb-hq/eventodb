package integration

import (
	"fmt"
	"testing"
)

// TestCATEGORY001_ReadFromCategory tests reading messages from multiple streams in a category
// Per SDK-TEST-SPEC.md: Write messages to test-1, test-2, test-3 streams
func TestCATEGORY001_ReadFromCategory(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	// Write messages to test-1, test-2, test-3 streams
	for i := 1; i <= 3; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get category messages
	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}
}

// TestCATEGORY002_ReadCategoryWithPositionFilter tests category reading with global position filter
func TestCATEGORY002_ReadCategoryWithPositionFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	// Write 4 messages and track global positions
	var globalPositions []int64
	for i := 1; i <= 4; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}

		resultMap := result.(map[string]interface{})
		gpos := int64(resultMap["globalPosition"].(float64))
		globalPositions = append(globalPositions, gpos)
	}

	// Get target global position (3rd message)
	targetGpos := globalPositions[2]

	// Get messages from that position
	opts := map[string]interface{}{
		"position": targetGpos,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category, opts)
	if err != nil {
		t.Fatalf("Failed to get category messages with position filter: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get messages from position onwards (3rd and 4th = 2 messages)
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages from position %d, got %d", targetGpos, len(messages))
	}

	// Verify all returned messages have globalPosition >= targetGpos
	for _, msgInterface := range messages {
		msgArray := msgInterface.([]interface{})
		if len(msgArray) < 5 {
			t.Errorf("Message array too short: %d elements", len(msgArray))
			continue
		}
		msgGpos := int64(msgArray[4].(float64))
		if msgGpos < targetGpos {
			t.Errorf("Message globalPosition %d is less than target %d", msgGpos, targetGpos)
		}
	}
}

// TestCATEGORY003_ReadCategoryWithBatchSize tests category reading with batch size limit
func TestCATEGORY003_ReadCategoryWithBatchSize(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	// Write 50 messages across multiple streams
	for i := 1; i <= 50; i++ {
		stream := fmt.Sprintf("%s-%d", category, i%10)
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get with batch size of 10
	opts := map[string]interface{}{
		"batchSize": 10,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category, opts)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	if len(messages) != 10 {
		t.Errorf("Expected exactly 10 messages, got %d", len(messages))
	}
}

// TestCATEGORY004_CategoryMessageFormat tests the message format returned by category queries
func TestCATEGORY004_CategoryMessageFormat(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	stream := fmt.Sprintf("%s-123", category)
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Check message format: [id, streamName, type, position, globalPosition, data, metadata, time]
	msgArray, ok := messages[0].([]interface{})
	if !ok {
		t.Fatalf("Expected message to be an array, got %T", messages[0])
	}

	if len(msgArray) != 8 {
		t.Errorf("Expected message array to have 8 elements, got %d", len(msgArray))
	}

	// Verify each field
	id, ok := msgArray[0].(string)
	if !ok || id == "" {
		t.Errorf("Expected id to be a non-empty string, got %v", msgArray[0])
	}

	streamName, ok := msgArray[1].(string)
	if !ok || streamName != stream {
		t.Errorf("Expected stream name to be %s, got %v", stream, msgArray[1])
	}

	msgType, ok := msgArray[2].(string)
	if !ok || msgType != "TestEvent" {
		t.Errorf("Expected type to be TestEvent, got %v", msgArray[2])
	}

	position, ok := msgArray[3].(float64)
	if !ok || int(position) != 0 {
		t.Errorf("Expected position to be 0, got %v", msgArray[3])
	}

	_, ok = msgArray[4].(float64)
	if !ok {
		t.Errorf("Expected globalPosition to be a number, got %T", msgArray[4])
	}

	data, ok := msgArray[5].(map[string]interface{})
	if !ok {
		t.Errorf("Expected data to be a map, got %T", msgArray[5])
	} else {
		if data["foo"] != "bar" {
			t.Errorf("Expected data.foo to be 'bar', got %v", data["foo"])
		}
	}

	// metadata can be map or nil
	if msgArray[6] != nil {
		if _, ok := msgArray[6].(map[string]interface{}); !ok {
			t.Errorf("Expected metadata to be a map or nil, got %T", msgArray[6])
		}
	}

	_, ok = msgArray[7].(string)
	if !ok {
		t.Errorf("Expected time to be a string, got %T", msgArray[7])
	}
}

// TestCATEGORY005_CategoryWithConsumerGroup tests consumer group filtering for categories
func TestCATEGORY005_CategoryWithConsumerGroup(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	// Write messages to test-1, test-2, test-3, test-4
	for i := 1; i <= 4; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", stream, err)
		}
	}

	// Get messages for consumer 0 of 2
	opts0 := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 0,
			"size":   2,
		},
	}

	result0, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category, opts0)
	if err != nil {
		t.Fatalf("Failed to get consumer 0 messages: %v", err)
	}

	messages0, ok := result0.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result0)
	}

	// Get messages for consumer 1 of 2
	opts1 := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 1,
			"size":   2,
		},
	}

	result1, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category, opts1)
	if err != nil {
		t.Fatalf("Failed to get consumer 1 messages: %v", err)
	}

	messages1, ok := result1.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result1)
	}

	// Both should have some messages
	if len(messages0) == 0 {
		t.Error("Consumer 0 should have messages")
	}
	if len(messages1) == 0 {
		t.Error("Consumer 1 should have messages")
	}

	// Together they should have all 4 messages
	total := len(messages0) + len(messages1)
	if total != 4 {
		t.Errorf("Expected total of 4 messages, got %d (consumer 0: %d, consumer 1: %d)",
			total, len(messages0), len(messages1))
	}
}

// TestCATEGORY006_CategoryWithCorrelationFilter tests correlation filtering for categories
func TestCATEGORY006_CategoryWithCorrelationFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())

	// Write message to test-1 with workflow correlation
	stream1 := fmt.Sprintf("%s-1", category)
	message1 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
		"metadata": map[string]interface{}{
			"correlationStreamName": "workflow-123",
		},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream1, message1)
	if err != nil {
		t.Fatalf("Failed to write message 1: %v", err)
	}

	// Write message to test-2 with other correlation
	stream2 := fmt.Sprintf("%s-2", category)
	message2 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
		"metadata": map[string]interface{}{
			"correlationStreamName": "other-456",
		},
	}

	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream2, message2)
	if err != nil {
		t.Fatalf("Failed to write message 2: %v", err)
	}

	// Filter by workflow correlation
	opts := map[string]interface{}{
		"correlation": "workflow",
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category, opts)
	if err != nil {
		t.Fatalf("Failed to get category messages with correlation: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get only the message with workflow correlation
	if len(messages) != 1 {
		t.Errorf("Expected 1 message with workflow correlation, got %d", len(messages))
	}

	if len(messages) > 0 {
		msgArray := messages[0].([]interface{})
		streamName := msgArray[1].(string)
		if streamName != stream1 {
			t.Errorf("Expected message from stream %s, got %s", stream1, streamName)
		}
	}
}

// TestCATEGORY007_ReadFromEmptyCategory tests reading from a non-existent category
func TestCATEGORY007_ReadFromEmptyCategory(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("nonexistent%d", uniqueSuffix())

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	if len(messages) != 0 {
		t.Errorf("Expected empty array for nonexistent category, got %d messages", len(messages))
	}
}

// TestCATEGORY008_CategoryGlobalPositionOrdering tests that category messages are returned in global position order
func TestCATEGORY008_CategoryGlobalPositionOrdering(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	category := fmt.Sprintf("test%d", uniqueSuffix())
	message := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{},
	}

	// Write messages across multiple streams
	for i := 1; i <= 10; i++ {
		stream := fmt.Sprintf("%s-%d", category, i%3)
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, message)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", category)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Extract global positions
	var globalPositions []int64
	for _, msgInterface := range messages {
		msgArray := msgInterface.([]interface{})
		gpos := int64(msgArray[4].(float64))
		globalPositions = append(globalPositions, gpos)
	}

	// Verify they are in ascending order
	for i := 1; i < len(globalPositions); i++ {
		if globalPositions[i] < globalPositions[i-1] {
			t.Errorf("Global positions not in ascending order: %d at index %d is less than %d at index %d",
				globalPositions[i], i, globalPositions[i-1], i-1)
		}
	}
}

// Helper function to generate unique suffix for test isolation
func uniqueSuffix() int64 {
	return int64(1000000 + len("test"))
}
