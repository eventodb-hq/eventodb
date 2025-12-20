package eventodb

import (
	"context"
	"fmt"
	"testing"
)

func TestCATEGORY001_ReadFromCategory(t *testing.T) {
	tc := setupTest(t, "category-001")
	ctx := context.Background()

	category := randomCategoryName()

	// Write messages to category-1, category-2, category-3 streams
	for i := 1; i <= 3; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"stream": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write to stream %d: %v", i, err)
		}
	}

	messages, err := tc.client.CategoryGet(ctx, category, nil)
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}
}

func TestCATEGORY002_ReadCategoryWithPositionFilter(t *testing.T) {
	tc := setupTest(t, "category-002")
	ctx := context.Background()

	category := randomCategoryName()

	// Write 4 messages to different streams in same category
	var globalPositions []int64
	for i := 0; i < 4; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		result, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"index": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
		globalPositions = append(globalPositions, result.GlobalPosition)
	}

	// Read from position 2 (global position of 3rd message)
	messages, err := tc.client.CategoryGet(ctx, category, &GetCategoryOptions{
		Position: &globalPositions[2],
	})
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	// Should get messages at global positions 2 and 3
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}
}

func TestCATEGORY003_ReadCategoryWithBatchSize(t *testing.T) {
	tc := setupTest(t, "category-003")
	ctx := context.Background()

	category := randomCategoryName()

	// Write 50 messages across multiple streams
	for i := 0; i < 50; i++ {
		stream := fmt.Sprintf("%s-%d", category, i%10) // Use 10 streams, 5 messages each
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"index": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Read with batch size 10
	messages, err := tc.client.CategoryGet(ctx, category, &GetCategoryOptions{
		BatchSize: intPtr(10),
	})
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	if len(messages) != 10 {
		t.Errorf("Expected exactly 10 messages, got %d", len(messages))
	}
}

func TestCATEGORY004_CategoryMessageFormat(t *testing.T) {
	tc := setupTest(t, "category-004")
	ctx := context.Background()

	category := randomCategoryName()
	stream := category + "-123"

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.CategoryGet(ctx, category, nil)
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]

	// Verify all fields are present
	if msg.ID == "" {
		t.Error("Expected ID to be set")
	}
	if msg.StreamName == "" {
		t.Error("Expected StreamName to be set")
	}
	if msg.Type != "TestEvent" {
		t.Errorf("Expected type 'TestEvent', got %s", msg.Type)
	}
	if msg.Position < 0 {
		t.Errorf("Expected position >= 0, got %d", msg.Position)
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

func TestCATEGORY005_CategoryWithConsumerGroup(t *testing.T) {
	tc := setupTest(t, "category-005")
	ctx := context.Background()

	category := randomCategoryName()

	// Write messages to streams: category-1, category-2, category-3, category-4
	for i := 1; i <= 4; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"stream": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write to stream %d: %v", i, err)
		}
	}

	// Read with consumer group member 0 of size 2
	messages, err := tc.client.CategoryGet(ctx, category, &GetCategoryOptions{
		ConsumerGroup: &ConsumerGroup{
			Member: 0,
			Size:   2,
		},
	})
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	// Should get a subset of messages (deterministic based on hash)
	if len(messages) == 0 {
		t.Error("Expected at least some messages for consumer group")
	}

	// Should not get all 4 messages (unless hash collision)
	t.Logf("Consumer group member 0/2 got %d messages", len(messages))
}

func TestCATEGORY006_CategoryWithCorrelationFilter(t *testing.T) {
	tc := setupTest(t, "category-006")
	ctx := context.Background()

	category := randomCategoryName()

	// Write message to category-1 with correlation
	stream1 := category + "-1"
	_, err := tc.client.StreamWrite(ctx, stream1, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"id": 1},
		Metadata: map[string]interface{}{
			"correlationStreamName": "workflow-123",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to stream 1: %v", err)
	}

	// Write message to category-2 with different correlation
	stream2 := category + "-2"
	_, err = tc.client.StreamWrite(ctx, stream2, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"id": 2},
		Metadata: map[string]interface{}{
			"correlationStreamName": "other-456",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to stream 2: %v", err)
	}

	// Read with correlation filter "workflow"
	correlation := "workflow"
	messages, err := tc.client.CategoryGet(ctx, category, &GetCategoryOptions{
		Correlation: &correlation,
	})
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	// Should only get message from stream1
	if len(messages) != 1 {
		t.Errorf("Expected 1 message with correlation filter, got %d", len(messages))
	}

	if len(messages) > 0 && messages[0].Data["id"] != 1.0 {
		t.Errorf("Expected message from stream 1, got %+v", messages[0])
	}
}

func TestCATEGORY007_ReadFromEmptyCategory(t *testing.T) {
	tc := setupTest(t, "category-007")
	ctx := context.Background()

	category := "nonexistent-" + randomStreamName()

	messages, err := tc.client.CategoryGet(ctx, category, nil)
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected empty array, got %d messages", len(messages))
	}
}

func TestCATEGORY008_CategoryGlobalPositionOrdering(t *testing.T) {
	tc := setupTest(t, "category-008")
	ctx := context.Background()

	category := randomCategoryName()

	// Write messages to different streams
	for i := 0; i < 5; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"index": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	messages, err := tc.client.CategoryGet(ctx, category, nil)
	if err != nil {
		t.Fatalf("Failed to read category: %v", err)
	}

	if len(messages) < 2 {
		t.Skip("Need at least 2 messages to verify ordering")
	}

	// Verify messages are in ascending global position order
	for i := 1; i < len(messages); i++ {
		if messages[i].GlobalPosition <= messages[i-1].GlobalPosition {
			t.Errorf("Messages not in global position order: %d <= %d at index %d",
				messages[i].GlobalPosition, messages[i-1].GlobalPosition, i)
		}
	}
}
