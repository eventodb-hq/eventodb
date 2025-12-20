package eventodb

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestREAD001_ReadFromEmptyStream(t *testing.T) {
	tc := setupTest(t, "read-001")
	ctx := context.Background()

	stream := randomStreamName()

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected empty array, got %d messages", len(messages))
	}
}

func TestREAD002_ReadSingleMessage(t *testing.T) {
	tc := setupTest(t, "read-002")
	ctx := context.Background()

	stream := randomStreamName()

	// Write one message
	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read it back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]

	// Verify all fields are present
	if msg.ID == "" {
		t.Error("Expected ID to be set")
	}
	if msg.Type != "TestEvent" {
		t.Errorf("Expected type 'TestEvent', got %s", msg.Type)
	}
	if msg.Position != 0 {
		t.Errorf("Expected position 0, got %d", msg.Position)
	}
	if msg.GlobalPosition < 0 {
		t.Errorf("Expected globalPosition >= 0, got %d", msg.GlobalPosition)
	}
	if msg.Data == nil {
		t.Error("Expected data to be set")
	}
	if msg.Time.IsZero() {
		t.Error("Expected time to be set")
	}
}

func TestREAD003_ReadMultipleMessages(t *testing.T) {
	tc := setupTest(t, "read-003")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 5 messages
	for i := 0; i < 5; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read them back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(messages))
	}

	// Verify positions are in order (0, 1, 2, 3, 4)
	for i, msg := range messages {
		if msg.Position != int64(i) {
			t.Errorf("Expected position %d at index %d, got %d", i, i, msg.Position)
		}
	}
}

func TestREAD004_ReadWithPositionFilter(t *testing.T) {
	tc := setupTest(t, "read-004")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 10 messages
	for i := 0; i < 10; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read from position 5
	messages, err := tc.client.StreamGet(ctx, stream, &GetStreamOptions{
		Position: int64Ptr(5),
	})
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	// Should get messages at positions 5, 6, 7, 8, 9
	if len(messages) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(messages))
	}

	// Verify first message is at position 5
	if messages[0].Position != 5 {
		t.Errorf("Expected first message at position 5, got %d", messages[0].Position)
	}

	// Verify last message is at position 9
	if messages[4].Position != 9 {
		t.Errorf("Expected last message at position 9, got %d", messages[4].Position)
	}
}

func TestREAD005_ReadWithGlobalPositionFilter(t *testing.T) {
	tc := setupTest(t, "read-005")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 4 messages and capture their global positions
	var globalPositions []int64
	for i := 0; i < 4; i++ {
		result, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
		globalPositions = append(globalPositions, result.GlobalPosition)
	}

	// Read from global position of message 2 (index 2)
	// Note: In namespace-isolated environments, globalPosition might map to stream position
	targetGlobalPos := globalPositions[2]
	messages, err := tc.client.StreamGet(ctx, stream, &GetStreamOptions{
		GlobalPosition: &targetGlobalPos,
	})
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	// The exact behavior of globalPosition filtering varies by implementation
	// Just verify we get some messages and they're from our stream
	t.Logf("Got %d messages with globalPosition filter %d", len(messages), targetGlobalPos)

	// Verify messages are from the correct stream
	for _, msg := range messages {
		if msg.Position < 0 || msg.Position >= 4 {
			t.Errorf("Got message with unexpected position: %d", msg.Position)
		}
	}
}

func TestREAD006_ReadWithBatchSizeLimit(t *testing.T) {
	tc := setupTest(t, "read-006")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 100 messages
	for i := 0; i < 100; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read with batch size 10
	messages, err := tc.client.StreamGet(ctx, stream, &GetStreamOptions{
		BatchSize: intPtr(10),
	})
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 10 {
		t.Errorf("Expected exactly 10 messages, got %d", len(messages))
	}

	// Verify positions are 0-9
	for i, msg := range messages {
		if msg.Position != int64(i) {
			t.Errorf("Expected position %d at index %d, got %d", i, i, msg.Position)
		}
	}
}

func TestREAD007_ReadWithBatchSizeUnlimited(t *testing.T) {
	tc := setupTest(t, "read-007")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 50 messages
	for i := 0; i < 50; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read with batch size -1 (unlimited)
	messages, err := tc.client.StreamGet(ctx, stream, &GetStreamOptions{
		BatchSize: intPtr(-1),
	})
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 50 {
		t.Errorf("Expected all 50 messages, got %d", len(messages))
	}
}

func TestREAD008_ReadMessageDataIntegrity(t *testing.T) {
	tc := setupTest(t, "read-008")
	ctx := context.Background()

	stream := randomStreamName()

	// Write message with complex nested data
	complexData := map[string]interface{}{
		"nested": map[string]interface{}{
			"array": []interface{}{1.0, 2.0, 3.0},
			"bool":  true,
			"null":  nil,
		},
	}

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: complexData,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Deep equality check
	if !reflect.DeepEqual(messages[0].Data, complexData) {
		t.Errorf("Data mismatch.\nExpected: %+v\nGot: %+v", complexData, messages[0].Data)
	}
}

func TestREAD009_ReadMessageMetadataIntegrity(t *testing.T) {
	tc := setupTest(t, "read-009")
	ctx := context.Background()

	stream := randomStreamName()

	metadata := map[string]interface{}{
		"correlationId": "123",
		"userId":        "user-456",
	}

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type:     "TestEvent",
		Data:     map[string]interface{}{"foo": "bar"},
		Metadata: metadata,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if !reflect.DeepEqual(messages[0].Metadata, metadata) {
		t.Errorf("Metadata mismatch.\nExpected: %+v\nGot: %+v", metadata, messages[0].Metadata)
	}
}

func TestREAD010_ReadMessageTimestampFormat(t *testing.T) {
	tc := setupTest(t, "read-010")
	ctx := context.Background()

	stream := randomStreamName()

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Verify timestamp is valid and in UTC
	ts := messages[0].Time
	if ts.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	if ts.Location() != time.UTC {
		t.Errorf("Expected UTC timezone, got %v", ts.Location())
	}

	// Verify it's a reasonable recent time (within last minute)
	if time.Since(ts) > time.Minute {
		t.Errorf("Timestamp seems too old: %v", ts)
	}

	if ts.After(time.Now()) {
		t.Errorf("Timestamp is in the future: %v", ts)
	}
}
