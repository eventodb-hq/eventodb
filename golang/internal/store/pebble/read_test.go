package pebble

import (
	"context"
	"testing"
	"time"

	"github.com/message-db/message-db/internal/store"
)

func TestGetStreamMessages(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write messages to stream
	streamName := "account-123"
	for i := 0; i < 10; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountCreated",
			Data:       map[string]interface{}{"index": i},
		}
		_, err := s.WriteMessage(ctx, "test", streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Test: Get all messages
	msgs, err := s.GetStreamMessages(ctx, "test", streamName, &store.GetOpts{
		Position:  0,
		BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("failed to get stream messages: %v", err)
	}

	if len(msgs) != 10 {
		t.Errorf("expected 10 messages, got %d", len(msgs))
	}

	// Verify positions are sequential
	for i, msg := range msgs {
		if msg.Position != int64(i) {
			t.Errorf("message %d: expected position %d, got %d", i, i, msg.Position)
		}
		if msg.StreamName != streamName {
			t.Errorf("message %d: expected stream %s, got %s", i, streamName, msg.StreamName)
		}
		if msg.Type != "AccountCreated" {
			t.Errorf("message %d: expected type AccountCreated, got %s", i, msg.Type)
		}
	}

	// Test: Pagination
	msgs, err = s.GetStreamMessages(ctx, "test", streamName, &store.GetOpts{
		Position:  5,
		BatchSize: 3,
	})
	if err != nil {
		t.Fatalf("failed to get stream messages with pagination: %v", err)
	}

	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Position != 5 {
		t.Errorf("expected first message position 5, got %d", msgs[0].Position)
	}
}

func TestGetCategoryMessages(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write messages to multiple streams in same category
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: "account-123",
			Type:       "AccountEvent",
			Data:       map[string]interface{}{"index": i},
		}
		_, err := s.WriteMessage(ctx, "test", "account-123", msg)
		if err != nil {
			t.Fatalf("failed to write message to account-123: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: "account-456",
			Type:       "AccountEvent",
			Data:       map[string]interface{}{"index": i + 100},
		}
		_, err := s.WriteMessage(ctx, "test", "account-456", msg)
		if err != nil {
			t.Fatalf("failed to write message to account-456: %v", err)
		}
	}

	// Test: Get all category messages
	msgs, err := s.GetCategoryMessages(ctx, "test", "account", &store.CategoryOpts{
		Position:  1,
		BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("failed to get category messages: %v", err)
	}

	if len(msgs) != 10 {
		t.Errorf("expected 10 messages, got %d", len(msgs))
	}

	// Verify messages are from both streams
	hasStream123 := false
	hasStream456 := false
	for _, msg := range msgs {
		if msg.StreamName == "account-123" {
			hasStream123 = true
		}
		if msg.StreamName == "account-456" {
			hasStream456 = true
		}
	}

	if !hasStream123 || !hasStream456 {
		t.Errorf("expected messages from both streams, got 123=%v 456=%v", hasStream123, hasStream456)
	}

	// Test: Pagination
	msgs, err = s.GetCategoryMessages(ctx, "test", "account", &store.CategoryOpts{
		Position:  1,
		BatchSize: 5,
	})
	if err != nil {
		t.Fatalf("failed to get category messages with pagination: %v", err)
	}

	if len(msgs) != 5 {
		t.Errorf("expected 5 messages, got %d", len(msgs))
	}
}

func TestGetCategoryMessages_ConsumerGroup(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write messages to multiple streams (different cardinal IDs)
	cardinalIDs := []string{"1", "2", "3", "4", "5"}
	for _, id := range cardinalIDs {
		for i := 0; i < 2; i++ {
			msg := &store.Message{
				StreamName: "account-" + id,
				Type:       "AccountEvent",
				Data:       map[string]interface{}{"cardinalID": id, "index": i},
			}
			_, err := s.WriteMessage(ctx, "test", "account-"+id, msg)
			if err != nil {
				t.Fatalf("failed to write message: %v", err)
			}
		}
	}

	// Test: Consumer group with 2 members
	consumerSize := int64(2)
	member0 := int64(0)
	member1 := int64(1)

	msgs0, err := s.GetCategoryMessages(ctx, "test", "account", &store.CategoryOpts{
		Position:       1,
		BatchSize:      100,
		ConsumerMember: &member0,
		ConsumerSize:   &consumerSize,
	})
	if err != nil {
		t.Fatalf("failed to get messages for member 0: %v", err)
	}

	msgs1, err := s.GetCategoryMessages(ctx, "test", "account", &store.CategoryOpts{
		Position:       1,
		BatchSize:      100,
		ConsumerMember: &member1,
		ConsumerSize:   &consumerSize,
	})
	if err != nil {
		t.Fatalf("failed to get messages for member 1: %v", err)
	}

	// Verify that members get different messages
	if len(msgs0) == 0 || len(msgs1) == 0 {
		t.Errorf("both members should get messages: member0=%d, member1=%d", len(msgs0), len(msgs1))
	}

	// Verify total messages equals original count
	if len(msgs0)+len(msgs1) != 10 {
		t.Errorf("expected total 10 messages, got %d", len(msgs0)+len(msgs1))
	}

	// Verify no overlap
	streams0 := make(map[string]bool)
	for _, msg := range msgs0 {
		streams0[msg.StreamName] = true
	}
	for _, msg := range msgs1 {
		if streams0[msg.StreamName] {
			t.Errorf("stream %s appears in both member 0 and member 1", msg.StreamName)
		}
	}
}

func TestGetCategoryMessages_Correlation(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write messages with different correlation streams
	for i := 0; i < 3; i++ {
		correlation := "user-alice"
		msg := &store.Message{
			StreamName: "order-100",
			Type:       "OrderPlaced",
			Data:       map[string]interface{}{"index": i},
			Metadata:   map[string]interface{}{"correlationStreamName": correlation},
		}
		_, err := s.WriteMessage(ctx, "test", "order-100", msg)
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		correlation := "user-bob"
		msg := &store.Message{
			StreamName: "order-200",
			Type:       "OrderPlaced",
			Data:       map[string]interface{}{"index": i},
			Metadata:   map[string]interface{}{"correlationStreamName": correlation},
		}
		_, err := s.WriteMessage(ctx, "test", "order-200", msg)
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}

	// Test: Filter by correlation category (should match both user-alice and user-bob)
	correlationFilter := "user"
	msgs, err := s.GetCategoryMessages(ctx, "test", "order", &store.CategoryOpts{
		Position:    1,
		BatchSize:   100,
		Correlation: &correlationFilter,
	})
	if err != nil {
		t.Fatalf("failed to get category messages with correlation: %v", err)
	}

	// Should get all 5 messages since both user-alice and user-bob are in "user" category
	if len(msgs) != 5 {
		t.Errorf("expected 5 messages with correlation category user, got %d", len(msgs))
	}

	// Verify all messages have correct correlation category
	for _, msg := range msgs {
		if msg.Metadata == nil {
			t.Errorf("message has no metadata")
			continue
		}
		corrVal, ok := msg.Metadata["correlationStreamName"]
		if !ok {
			t.Errorf("message has no correlationStreamName")
			continue
		}
		corr, ok := corrVal.(string)
		if !ok {
			t.Errorf("correlationStreamName is not a string")
			continue
		}
		corrCat := extractCategory(corr)
		if corrCat != "user" {
			t.Errorf("expected correlation category user, got %s (from %s)", corrCat, corr)
		}
	}
}

func TestGetLastStreamMessage(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write messages with different types
	streamName := "account-123"
	types := []string{"Created", "Updated", "Updated", "Deleted"}
	for i, msgType := range types {
		msg := &store.Message{
			StreamName: streamName,
			Type:       msgType,
			Data:       map[string]interface{}{"index": i},
		}
		time.Sleep(time.Millisecond) // Ensure different timestamps
		_, err := s.WriteMessage(ctx, "test", streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}

	// Test: Get last message (any type)
	lastMsg, err := s.GetLastStreamMessage(ctx, "test", streamName, nil)
	if err != nil {
		t.Fatalf("failed to get last stream message: %v", err)
	}

	if lastMsg.Type != "Deleted" {
		t.Errorf("expected last message type Deleted, got %s", lastMsg.Type)
	}
	if lastMsg.Position != 3 {
		t.Errorf("expected last message position 3, got %d", lastMsg.Position)
	}

	// Test: Get last message of specific type
	msgType := "Updated"
	lastUpdated, err := s.GetLastStreamMessage(ctx, "test", streamName, &msgType)
	if err != nil {
		t.Fatalf("failed to get last updated message: %v", err)
	}

	if lastUpdated.Type != "Updated" {
		t.Errorf("expected message type Updated, got %s", lastUpdated.Type)
	}
	if lastUpdated.Position != 2 {
		t.Errorf("expected position 2, got %d", lastUpdated.Position)
	}

	// Test: Get last message of non-existent type
	msgType = "NonExistent"
	_, err = s.GetLastStreamMessage(ctx, "test", streamName, &msgType)
	if err != store.ErrStreamNotFound {
		t.Errorf("expected ErrStreamNotFound, got %v", err)
	}
}

func TestGetLastStreamMessage_StreamNotFound(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Test: Get last message from non-existent stream
	_, err = s.GetLastStreamMessage(ctx, "test", "nonexistent-stream", nil)
	if err != store.ErrStreamNotFound {
		t.Errorf("expected ErrStreamNotFound, got %v", err)
	}
}

func TestGetStreamVersion(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "account-123"

	// Test: Get version of non-existent stream
	version, err := s.GetStreamVersion(ctx, "test", streamName)
	if err != nil {
		t.Fatalf("failed to get stream version: %v", err)
	}
	if version != -1 {
		t.Errorf("expected version -1 for non-existent stream, got %d", version)
	}

	// Write messages
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountEvent",
			Data:       map[string]interface{}{"index": i},
		}
		_, err := s.WriteMessage(ctx, "test", streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message: %v", err)
		}
	}

	// Test: Get version of existing stream
	version, err = s.GetStreamVersion(ctx, "test", streamName)
	if err != nil {
		t.Fatalf("failed to get stream version: %v", err)
	}
	if version != 4 {
		t.Errorf("expected version 4, got %d", version)
	}
}

func TestGetStreamMessages_EmptyStream(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create namespace
	if err := s.CreateNamespace(ctx, "test", "secret123", "Test namespace"); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Test: Get messages from non-existent stream
	msgs, err := s.GetStreamMessages(ctx, "test", "nonexistent-stream", &store.GetOpts{
		Position:  0,
		BatchSize: 100,
	})
	if err != nil {
		t.Fatalf("failed to get stream messages: %v", err)
	}

	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetStreamMessages_InvalidNamespace(t *testing.T) {
	// Setup
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Test: Get messages from non-existent namespace
	_, err = s.GetStreamMessages(ctx, "nonexistent", "some-stream", &store.GetOpts{
		Position:  0,
		BatchSize: 100,
	})
	if err == nil {
		t.Error("expected error for non-existent namespace, got nil")
	}
}
