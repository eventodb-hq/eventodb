package integration

import (
	"testing"
)

// TestMDB002_4A_T1: Test get category returns messages from multiple streams
func TestMDB002_4A_T1_GetCategoryReturnsMessagesFromMultipleStreams(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write messages to multiple streams in the same category
	msg1 := map[string]interface{}{
		"type": "AccountCreated",
		"data": map[string]interface{}{"accountId": "123", "balance": 0},
	}
	msg2 := map[string]interface{}{
		"type": "AccountCreated",
		"data": map[string]interface{}{"accountId": "456", "balance": 0},
	}
	msg3 := map[string]interface{}{
		"type": "Deposited",
		"data": map[string]interface{}{"amount": 100},
	}

	// Write to account-123
	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-123", msg1)
	if err != nil {
		t.Fatalf("Failed to write message 1: %v", err)
	}

	// Write to account-456
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-456", msg2)
	if err != nil {
		t.Fatalf("Failed to write message 2: %v", err)
	}

	// Write another message to account-123
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-123", msg3)
	if err != nil {
		t.Fatalf("Failed to write message 3: %v", err)
	}

	// Get category messages
	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account")
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get 3 messages
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	// Verify we have messages from both streams
	streamNames := make(map[string]bool)
	for _, msgInterface := range messages {
		msgArray, ok := msgInterface.([]interface{})
		if !ok {
			t.Errorf("Expected message to be an array, got %T", msgInterface)
			continue
		}
		if len(msgArray) < 8 {
			t.Errorf("Expected message array to have 8 elements, got %d", len(msgArray))
			continue
		}
		streamName, ok := msgArray[1].(string)
		if !ok {
			t.Errorf("Expected stream name to be a string, got %T", msgArray[1])
			continue
		}
		streamNames[streamName] = true
	}

	if !streamNames["account-123"] || !streamNames["account-456"] {
		t.Errorf("Expected messages from both account-123 and account-456, got %v", streamNames)
	}
}

// TestMDB002_4A_T2: Test category includes stream names in response
func TestMDB002_4A_T2_CategoryIncludesStreamNamesInResponse(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write a message
	msg := map[string]interface{}{
		"type": "AccountCreated",
		"data": map[string]interface{}{"accountId": "123"},
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-123", msg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Get category messages
	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account")
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	if len(messages) == 0 {
		t.Fatal("Expected at least one message")
	}

	// Check message format: [id, streamName, type, position, globalPosition, data, metadata, time]
	msgArray, ok := messages[0].([]interface{})
	if !ok {
		t.Fatalf("Expected message to be an array, got %T", messages[0])
	}

	if len(msgArray) != 8 {
		t.Errorf("Expected message array to have 8 elements, got %d", len(msgArray))
	}

	// Verify stream name is at index 1
	streamName, ok := msgArray[1].(string)
	if !ok {
		t.Errorf("Expected stream name to be a string, got %T", msgArray[1])
	}

	if streamName != "account-123" {
		t.Errorf("Expected stream name to be 'account-123', got '%s'", streamName)
	}

	// Verify type is at index 2
	msgType, ok := msgArray[2].(string)
	if !ok {
		t.Errorf("Expected type to be a string, got %T", msgArray[2])
	}

	if msgType != "AccountCreated" {
		t.Errorf("Expected type to be 'AccountCreated', got '%s'", msgType)
	}
}

// TestMDB002_4A_T3: Test category with position filter
func TestMDB002_4A_T3_CategoryWithPositionFilter(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write multiple messages
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "Event",
			"data": map[string]interface{}{"index": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-123", msg)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get all messages to find the global position of the 3rd message
	allResult, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account")
	if err != nil {
		t.Fatalf("Failed to get all messages: %v", err)
	}

	allMessages, ok := allResult.([]interface{})
	if !ok || len(allMessages) < 3 {
		t.Fatalf("Expected at least 3 messages, got %d", len(allMessages))
	}

	// Get global position of 3rd message
	thirdMsg := allMessages[2].([]interface{})
	thirdGlobalPos := int64(thirdMsg[4].(float64))

	// Get messages from that position
	opts := map[string]interface{}{
		"position": thirdGlobalPos,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages with position filter: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get messages from position onwards (3rd, 4th, 5th = 3 messages)
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages from position %d, got %d", thirdGlobalPos, len(messages))
	}
}

// TestMDB002_4A_T4: Test category with batchSize limit
func TestMDB002_4A_T4_CategoryWithBatchSizeLimit(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write 10 messages
	for i := 0; i < 10; i++ {
		msg := map[string]interface{}{
			"type": "Event",
			"data": map[string]interface{}{"index": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-123", msg)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get with batch size of 5
	opts := map[string]interface{}{
		"batchSize": 5,
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get exactly 5 messages
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}
}

// TestMDB002_4A_T5: Test category with consumer group (member 0 of 2)
func TestMDB002_4A_T5_CategoryWithConsumerGroup(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write messages to different streams
	// account-100, account-200, account-300, account-400
	for i := 1; i <= 4; i++ {
		msg := map[string]interface{}{
			"type": "Event",
			"data": map[string]interface{}{"index": i},
		}
		streamName := "account-" + string(rune('0'+i)) + "00"
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", streamName, msg)
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", streamName, err)
		}
	}

	// Get messages for consumer 0 of 2
	opts := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 0,
			"size":   2,
		},
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages0, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Get messages for consumer 1 of 2
	opts["consumerGroup"] = map[string]interface{}{
		"member": 1,
		"size":   2,
	}

	result, err = makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages1, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Both consumers should have messages
	if len(messages0) == 0 {
		t.Error("Consumer 0 should have messages")
	}
	if len(messages1) == 0 {
		t.Error("Consumer 1 should have messages")
	}

	// Total should be 4 messages
	total := len(messages0) + len(messages1)
	if total != 4 {
		t.Errorf("Expected total of 4 messages, got %d (consumer 0: %d, consumer 1: %d)",
			total, len(messages0), len(messages1))
	}
}

// TestMDB002_4A_T6: Test consumer groups have no overlap
func TestMDB002_4A_T6_ConsumerGroupsHaveNoOverlap(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write messages to multiple streams
	for i := 1; i <= 10; i++ {
		msg := map[string]interface{}{
			"type": "Event",
			"data": map[string]interface{}{"index": i},
		}
		streamName := "account-" + string(rune('0'+i))
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", streamName, msg)
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", streamName, err)
		}
	}

	// Get messages for all 3 consumers
	consumer0Opts := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 0,
			"size":   3,
		},
	}
	consumer1Opts := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 1,
			"size":   3,
		},
	}
	consumer2Opts := map[string]interface{}{
		"consumerGroup": map[string]interface{}{
			"member": 2,
			"size":   3,
		},
	}

	result0, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", consumer0Opts)
	if err != nil {
		t.Fatalf("Failed to get consumer 0 messages: %v", err)
	}
	messages0 := result0.([]interface{})

	result1, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", consumer1Opts)
	if err != nil {
		t.Fatalf("Failed to get consumer 1 messages: %v", err)
	}
	messages1 := result1.([]interface{})

	result2, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", consumer2Opts)
	if err != nil {
		t.Fatalf("Failed to get consumer 2 messages: %v", err)
	}
	messages2 := result2.([]interface{})

	// Collect all stream names from each consumer
	getStreamNames := func(messages []interface{}) map[string]bool {
		streams := make(map[string]bool)
		for _, msgInterface := range messages {
			msgArray := msgInterface.([]interface{})
			streamName := msgArray[1].(string)
			streams[streamName] = true
		}
		return streams
	}

	streams0 := getStreamNames(messages0)
	streams1 := getStreamNames(messages1)
	streams2 := getStreamNames(messages2)

	// Check for overlap
	for stream := range streams0 {
		if streams1[stream] {
			t.Errorf("Consumer 0 and 1 both have messages from stream: %s", stream)
		}
		if streams2[stream] {
			t.Errorf("Consumer 0 and 2 both have messages from stream: %s", stream)
		}
	}
	for stream := range streams1 {
		if streams2[stream] {
			t.Errorf("Consumer 1 and 2 both have messages from stream: %s", stream)
		}
	}

	// Total messages should be 10
	total := len(messages0) + len(messages1) + len(messages2)
	if total != 10 {
		t.Errorf("Expected total of 10 messages, got %d", total)
	}
}

// TestMDB002_4A_T7: Test correlation filtering
func TestMDB002_4A_T7_CorrelationFiltering(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write messages with different correlation metadata
	msg1 := map[string]interface{}{
		"type": "Event",
		"data": map[string]interface{}{"index": 1},
		"metadata": map[string]interface{}{
			"correlationStreamName": "workflow-123",
		},
	}
	msg2 := map[string]interface{}{
		"type": "Event",
		"data": map[string]interface{}{"index": 2},
		"metadata": map[string]interface{}{
			"correlationStreamName": "workflow-456",
		},
	}
	msg3 := map[string]interface{}{
		"type": "Event",
		"data": map[string]interface{}{"index": 3},
		"metadata": map[string]interface{}{
			"correlationStreamName": "process-789",
		},
	}
	msg4 := map[string]interface{}{
		"type": "Event",
		"data": map[string]interface{}{"index": 4},
		// No correlation metadata
	}

	_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-1", msg1)
	if err != nil {
		t.Fatalf("Failed to write message 1: %v", err)
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-2", msg2)
	if err != nil {
		t.Fatalf("Failed to write message 2: %v", err)
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-3", msg3)
	if err != nil {
		t.Fatalf("Failed to write message 3: %v", err)
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", "account-4", msg4)
	if err != nil {
		t.Fatalf("Failed to write message 4: %v", err)
	}

	// Get messages correlated with "workflow" category
	opts := map[string]interface{}{
		"correlation": "workflow",
	}

	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages with correlation: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get only messages with workflow correlation (2 messages)
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages with workflow correlation, got %d", len(messages))
	}

	// Verify correlation metadata
	for _, msgInterface := range messages {
		msgArray := msgInterface.([]interface{})
		metadata, ok := msgArray[6].(map[string]interface{})
		if !ok {
			t.Errorf("Expected metadata to be a map, got %T", msgArray[6])
			continue
		}
		corrStream, ok := metadata["correlationStreamName"].(string)
		if !ok {
			t.Error("Expected correlationStreamName in metadata")
			continue
		}
		if corrStream != "workflow-123" && corrStream != "workflow-456" {
			t.Errorf("Expected workflow correlation, got %s", corrStream)
		}
	}
}

// TestMDB002_4A_T8: Test empty category returns empty array
func TestMDB002_4A_T8_EmptyCategoryReturnsEmptyArray(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Don't write any messages to the category

	// Get category messages
	result, err := makeRPCCall(t, ts.Port, ts.Token, "category.get", "nonexistent")
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	messages, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected result to be an array, got %T", result)
	}

	// Should get empty array
	if len(messages) != 0 {
		t.Errorf("Expected empty array for nonexistent category, got %d messages", len(messages))
	}
}
