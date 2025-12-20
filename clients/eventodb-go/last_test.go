package eventodb

import (
	"context"
	"testing"
)

func TestLAST001_LastMessageFromNonEmptyStream(t *testing.T) {
	tc := setupTest(t, "last-001")
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

	// Get last message
	msg, err := tc.client.StreamLast(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if msg == nil {
		t.Fatal("Expected message, got nil")
	}

	if msg.Position != 4 {
		t.Errorf("Expected position 4 (last message), got %d", msg.Position)
	}

	if msg.Data["count"] != 4.0 {
		t.Errorf("Expected count 4, got %v", msg.Data["count"])
	}
}

func TestLAST002_LastMessageFromEmptyStream(t *testing.T) {
	tc := setupTest(t, "last-002")
	ctx := context.Background()

	stream := randomStreamName()

	msg, err := tc.client.StreamLast(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if msg != nil {
		t.Errorf("Expected nil for empty stream, got %+v", msg)
	}
}

func TestLAST003_LastMessageFilteredByType(t *testing.T) {
	tc := setupTest(t, "last-003")
	ctx := context.Background()

	stream := randomStreamName()

	// Write messages: TypeA, TypeB, TypeA, TypeB, TypeA at positions 0-4
	types := []string{"TypeA", "TypeB", "TypeA", "TypeB", "TypeA"}
	for i, msgType := range types {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: msgType,
			Data: map[string]interface{}{"index": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get last TypeB message
	typeB := "TypeB"
	msg, err := tc.client.StreamLast(ctx, stream, &GetLastOptions{
		Type: &typeB,
	})
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if msg == nil {
		t.Fatal("Expected message, got nil")
	}

	if msg.Type != "TypeB" {
		t.Errorf("Expected type TypeB, got %s", msg.Type)
	}

	if msg.Position != 3 {
		t.Errorf("Expected position 3 (last TypeB), got %d", msg.Position)
	}

	if msg.Data["index"] != 3.0 {
		t.Errorf("Expected index 3, got %v", msg.Data["index"])
	}
}

func TestLAST004_LastMessageTypeFilterNoMatch(t *testing.T) {
	tc := setupTest(t, "last-004")
	ctx := context.Background()

	stream := randomStreamName()

	// Write only TypeA messages
	for i := 0; i < 3; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TypeA",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Try to get last TypeB message
	typeB := "TypeB"
	msg, err := tc.client.StreamLast(ctx, stream, &GetLastOptions{
		Type: &typeB,
	})
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if msg != nil {
		t.Errorf("Expected nil (no match), got %+v", msg)
	}
}
