package eventodb

import (
	"context"
	"errors"
	"testing"
)

func TestWRITE001_WriteMinimalMessage(t *testing.T) {
	tc := setupTest(t, "write-001")
	ctx := context.Background()

	stream := randomStreamName()
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position < 0 {
		t.Errorf("Expected position >= 0, got %d", result.Position)
	}

	if result.GlobalPosition < 0 {
		t.Errorf("Expected globalPosition >= 0, got %d", result.GlobalPosition)
	}

	if result.Position != 0 {
		t.Errorf("Expected first message at position 0, got %d", result.Position)
	}
}

func TestWRITE002_WriteMessageWithMetadata(t *testing.T) {
	tc := setupTest(t, "write-002")
	ctx := context.Background()

	stream := randomStreamName()
	expectedMetadata := map[string]interface{}{"correlationId": "123"}

	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type:     "TestEvent",
		Data:     map[string]interface{}{"foo": "bar"},
		Metadata: expectedMetadata,
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Read back and verify metadata
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.Metadata == nil {
		t.Fatal("Expected metadata to be set")
	}

	if msg.Metadata["correlationId"] != "123" {
		t.Errorf("Expected correlationId '123', got %v", msg.Metadata["correlationId"])
	}
}

func TestWRITE003_WriteWithCustomMessageID(t *testing.T) {
	tc := setupTest(t, "write-003")
	ctx := context.Background()

	stream := randomStreamName()
	customID := "550e8400-e29b-41d4-a716-446655440000"

	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, &WriteOptions{
		ID: &customID,
	})

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Read back and verify ID
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].ID != customID {
		t.Errorf("Expected ID %s, got %s", customID, messages[0].ID)
	}
}

func TestWRITE004_WriteWithExpectedVersionSuccess(t *testing.T) {
	tc := setupTest(t, "write-004")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 2 messages to get to version 1
	for i := 0; i < 2; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Write with expected version 1 (should succeed)
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 2},
	}, &WriteOptions{
		ExpectedVersion: int64Ptr(1),
	})

	if err != nil {
		t.Fatalf("Failed to write with expected version: %v", err)
	}

	if result.Position != 2 {
		t.Errorf("Expected position 2, got %d", result.Position)
	}
}

func TestWRITE005_WriteWithExpectedVersionConflict(t *testing.T) {
	tc := setupTest(t, "write-005")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 2 messages to get to version 1
	for i := 0; i < 2; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Write with expected version 5 (should fail - conflict)
	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 2},
	}, &WriteOptions{
		ExpectedVersion: int64Ptr(5),
	})

	if err == nil {
		t.Fatal("Expected version conflict error, got nil")
	}

	var dbErr *Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("Expected Error type, got %T", err)
	}

	if dbErr.Code != "STREAM_VERSION_CONFLICT" {
		t.Errorf("Expected STREAM_VERSION_CONFLICT, got %s", dbErr.Code)
	}
}

func TestWRITE006_WriteMultipleMessagesSequentially(t *testing.T) {
	tc := setupTest(t, "write-006")
	ctx := context.Background()

	stream := randomStreamName()
	numMessages := 5

	var results []*WriteResult
	for i := 0; i < numMessages; i++ {
		result, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
		results = append(results, result)
	}

	// Verify positions are sequential (0, 1, 2, 3, 4)
	for i, result := range results {
		if result.Position != int64(i) {
			t.Errorf("Expected position %d, got %d", i, result.Position)
		}
	}

	// Verify global positions are monotonically increasing
	for i := 1; i < len(results); i++ {
		if results[i].GlobalPosition <= results[i-1].GlobalPosition {
			t.Errorf("Global positions not increasing: %d <= %d at index %d",
				results[i].GlobalPosition, results[i-1].GlobalPosition, i)
		}
	}
}

func TestWRITE007_WriteToStreamWithID(t *testing.T) {
	tc := setupTest(t, "write-007")
	ctx := context.Background()

	stream := "account-123"

	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Verify we can read it back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestWRITE008_WriteWithEmptyDataObject(t *testing.T) {
	tc := setupTest(t, "write-008")
	ctx := context.Background()

	stream := randomStreamName()

	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{},
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Read back and verify empty data
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Data == nil {
		t.Error("Expected data to be empty object, got nil")
	}

	if len(messages[0].Data) != 0 {
		t.Errorf("Expected empty data object, got %v", messages[0].Data)
	}
}

func TestWRITE009_WriteWithNullMetadata(t *testing.T) {
	tc := setupTest(t, "write-009")
	ctx := context.Background()

	stream := randomStreamName()

	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type:     "TestEvent",
		Data:     map[string]interface{}{"x": 1},
		Metadata: nil,
	}, nil)

	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Read back and verify null metadata
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Metadata should be nil or empty
	if messages[0].Metadata != nil && len(messages[0].Metadata) > 0 {
		t.Errorf("Expected nil or empty metadata, got %v", messages[0].Metadata)
	}
}
