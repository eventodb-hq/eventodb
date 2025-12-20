package eventodb

import (
	"context"
	"testing"
)

func TestVERSION001_VersionOfNonExistentStream(t *testing.T) {
	tc := setupTest(t, "version-001")
	ctx := context.Background()

	stream := randomStreamName()

	version, err := tc.client.StreamVersion(ctx, stream)
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}

	if version != nil {
		t.Logf("Got version: %v (expected nil)", *version)
		// Some implementations may return 0 instead of null for non-existent streams
		// Accept nil or a pointer to 0
		if *version != 0 {
			t.Errorf("Expected nil or 0 for non-existent stream, got %v", *version)
		}
	}
}

func TestVERSION002_VersionOfStreamWithMessages(t *testing.T) {
	tc := setupTest(t, "version-002")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 3 messages (positions 0, 1, 2)
	for i := 0; i < 3; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	version, err := tc.client.StreamVersion(ctx, stream)
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}

	if version == nil {
		t.Fatal("Expected version, got nil")
	}

	if *version != 2 {
		t.Errorf("Expected version 2 (last position), got %d", *version)
	}
}

func TestVERSION003_VersionAfterWrite(t *testing.T) {
	tc := setupTest(t, "version-003")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 1 message (position 0)
	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 0},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write first message: %v", err)
	}

	// Write another message (position 1)
	_, err = tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 1},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write second message: %v", err)
	}

	// Get version
	version, err := tc.client.StreamVersion(ctx, stream)
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}

	if version == nil {
		t.Fatal("Expected version, got nil")
	}

	if *version != 1 {
		t.Errorf("Expected version 1, got %d", *version)
	}
}
