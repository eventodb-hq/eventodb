package postgres

import (
	"context"
	"testing"

	"github.com/message-db/message-db/internal/store"
)

// MDB001_3A_T6: Test GetStreamMessages returns correct messages
func TestMDB001_3A_T6_GetStreamMessagesReturnsCorrectMessages(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-6"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-6", "Test Namespace 6")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "order-111"

	// Write some messages
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "OrderPlaced",
			Data: map[string]interface{}{
				"orderId": "111",
				"item":    i,
			},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Get all messages
	opts := store.NewGetOpts()
	messages, err := pgStore.GetStreamMessages(ctx, namespace, streamName, opts)
	if err != nil {
		t.Fatalf("failed to get stream messages: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(messages))
	}

	// Verify order
	for i, msg := range messages {
		if msg.Position != int64(i) {
			t.Errorf("message %d: expected position %d, got %d", i, i, msg.Position)
		}
		if msg.StreamName != streamName {
			t.Errorf("message %d: expected stream name %s, got %s", i, streamName, msg.StreamName)
		}
		if msg.Type != "OrderPlaced" {
			t.Errorf("message %d: expected type OrderPlaced, got %s", i, msg.Type)
		}
	}
}

// MDB001_3A_T7: Test GetStreamMessages with position offset and batch size
func TestMDB001_3A_T7_GetStreamMessagesWithPositionOffsetAndBatchSize(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-7"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-7", "Test Namespace 7")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "order-222"

	// Write 20 messages
	for i := 0; i < 20; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "OrderEvent",
			Data:       map[string]interface{}{"seq": i},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Get messages with offset and batch size
	opts := &store.GetOpts{
		Position:  5,
		BatchSize: 10,
	}

	messages, err := pgStore.GetStreamMessages(ctx, namespace, streamName, opts)
	if err != nil {
		t.Fatalf("failed to get stream messages: %v", err)
	}

	if len(messages) != 10 {
		t.Errorf("expected 10 messages, got %d", len(messages))
	}

	// Verify positions start at 5
	if messages[0].Position != 5 {
		t.Errorf("expected first message position 5, got %d", messages[0].Position)
	}

	if messages[9].Position != 14 {
		t.Errorf("expected last message position 14, got %d", messages[9].Position)
	}
}

// MDB001_3A_T8: Test GetCategoryMessages returns from multiple streams
func TestMDB001_3A_T8_GetCategoryMessagesReturnsFromMultipleStreams(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-8"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-8", "Test Namespace 8")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	category := "order"

	// Write messages to different streams in the same category
	streams := []string{"order-aaa", "order-bbb", "order-ccc"}
	totalMessages := 0

	for _, stream := range streams {
		for i := 0; i < 3; i++ {
			msg := &store.Message{
				StreamName: stream,
				Type:       "OrderEvent",
				Data:       map[string]interface{}{"stream": stream},
			}
			_, err := pgStore.WriteMessage(ctx, namespace, stream, msg)
			if err != nil {
				t.Fatalf("failed to write message to %s: %v", stream, err)
			}
			totalMessages++
		}
	}

	// Get all category messages
	opts := store.NewCategoryOpts()
	messages, err := pgStore.GetCategoryMessages(ctx, namespace, category, opts)
	if err != nil {
		t.Fatalf("failed to get category messages: %v", err)
	}

	if len(messages) != totalMessages {
		t.Errorf("expected %d messages, got %d", totalMessages, len(messages))
	}

	// Verify all streams are represented
	streamCounts := make(map[string]int)
	for _, msg := range messages {
		streamCounts[msg.StreamName]++
	}

	for _, stream := range streams {
		if streamCounts[stream] != 3 {
			t.Errorf("stream %s: expected 3 messages, got %d", stream, streamCounts[stream])
		}
	}
}

// MDB001_3A_T9: Test GetCategoryMessages with consumer groups
func TestMDB001_3A_T9_GetCategoryMessagesWithConsumerGroups(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-9"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-9", "Test Namespace 9")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	category := "account"

	// Write messages to multiple streams
	streams := []string{
		"account-001",
		"account-002",
		"account-003",
		"account-004",
		"account-005",
	}

	for _, stream := range streams {
		msg := &store.Message{
			StreamName: stream,
			Type:       "AccountEvent",
			Data:       map[string]interface{}{"stream": stream},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, stream, msg)
		if err != nil {
			t.Fatalf("failed to write message to %s: %v", stream, err)
		}
	}

	// Get messages for consumer group member 0 of 2
	consumerMember := int64(0)
	consumerSize := int64(2)
	opts := &store.CategoryOpts{
		Position:       1,
		BatchSize:      1000,
		ConsumerMember: &consumerMember,
		ConsumerSize:   &consumerSize,
	}

	messages0, err := pgStore.GetCategoryMessages(ctx, namespace, category, opts)
	if err != nil {
		t.Fatalf("failed to get category messages for member 0: %v", err)
	}

	// Get messages for consumer group member 1 of 2
	consumerMember = 1
	opts.ConsumerMember = &consumerMember

	messages1, err := pgStore.GetCategoryMessages(ctx, namespace, category, opts)
	if err != nil {
		t.Fatalf("failed to get category messages for member 1: %v", err)
	}

	// Verify partitioning
	totalMessages := len(messages0) + len(messages1)
	if totalMessages != len(streams) {
		t.Errorf("expected %d total messages across partitions, got %d", len(streams), totalMessages)
	}

	// Verify no overlap
	streams0 := make(map[string]bool)
	for _, msg := range messages0 {
		streams0[msg.StreamName] = true
	}

	for _, msg := range messages1 {
		if streams0[msg.StreamName] {
			t.Errorf("stream %s appears in both partitions", msg.StreamName)
		}
	}
}

// MDB001_3A_T10: Test GetCategoryMessages consumer group partition
func TestMDB001_3A_T10_GetCategoryMessagesConsumerGroupPartition(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-10"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-10", "Test Namespace 10")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	category := "product"

	// Write messages to 10 different streams
	for i := 0; i < 10; i++ {
		streamName := store.Category("product") + "-" + string(rune('A'+i))
		msg := &store.Message{
			StreamName: streamName,
			Type:       "ProductEvent",
			Data:       map[string]interface{}{"id": i},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message to %s: %v", streamName, err)
		}
	}

	// Test with 3 consumer group members
	consumerSize := int64(3)
	allMessages := make([][]*store.Message, 3)

	for member := int64(0); member < consumerSize; member++ {
		opts := &store.CategoryOpts{
			Position:       1,
			BatchSize:      1000,
			ConsumerMember: &member,
			ConsumerSize:   &consumerSize,
		}

		messages, err := pgStore.GetCategoryMessages(ctx, namespace, category, opts)
		if err != nil {
			t.Fatalf("failed to get messages for member %d: %v", member, err)
		}

		allMessages[member] = messages
	}

	// Verify all messages are accounted for
	totalMessages := 0
	seenStreams := make(map[string]int)

	for member, messages := range allMessages {
		totalMessages += len(messages)
		for _, msg := range messages {
			seenStreams[msg.StreamName]++
			if seenStreams[msg.StreamName] > 1 {
				t.Errorf("stream %s assigned to multiple members", msg.StreamName)
			}
		}
		t.Logf("Member %d got %d messages", member, len(messages))
	}

	if totalMessages != 10 {
		t.Errorf("expected 10 total messages, got %d", totalMessages)
	}
}

// MDB001_3A_T11: Test GetLastStreamMessage returns last
func TestMDB001_3A_T11_GetLastStreamMessageReturnsLast(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-11"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-11", "Test Namespace 11")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "user-555"

	// Write multiple messages
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "UserEvent",
			Data:       map[string]interface{}{"sequence": i},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Get last message
	lastMsg, err := pgStore.GetLastStreamMessage(ctx, namespace, streamName, nil)
	if err != nil {
		t.Fatalf("failed to get last stream message: %v", err)
	}

	if lastMsg == nil {
		t.Fatal("expected last message, got nil")
	}

	if lastMsg.Position != 4 {
		t.Errorf("expected position 4, got %d", lastMsg.Position)
	}

	if lastMsg.Data["sequence"].(float64) != 4 {
		t.Errorf("expected sequence 4, got %v", lastMsg.Data["sequence"])
	}
}

// MDB001_3A_T12: Test GetLastStreamMessage with type filter
func TestMDB001_3A_T12_GetLastStreamMessageWithTypeFilter(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-12"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-12", "Test Namespace 12")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "user-666"

	// Write messages with different types
	types := []string{"UserCreated", "UserUpdated", "UserUpdated", "UserDeleted"}
	for i, msgType := range types {
		msg := &store.Message{
			StreamName: streamName,
			Type:       msgType,
			Data:       map[string]interface{}{"sequence": i},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Get last UserUpdated message
	msgType := "UserUpdated"
	lastMsg, err := pgStore.GetLastStreamMessage(ctx, namespace, streamName, &msgType)
	if err != nil {
		t.Fatalf("failed to get last stream message: %v", err)
	}

	if lastMsg == nil {
		t.Fatal("expected last message, got nil")
	}

	if lastMsg.Type != "UserUpdated" {
		t.Errorf("expected type UserUpdated, got %s", lastMsg.Type)
	}

	if lastMsg.Position != 2 {
		t.Errorf("expected position 2, got %d", lastMsg.Position)
	}
}

// MDB001_3A_T13: Test GetStreamVersion returns correct version
func TestMDB001_3A_T13_GetStreamVersionReturnsCorrectVersion(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-13"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-13", "Test Namespace 13")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "cart-777"

	// Get version of empty stream
	version, err := pgStore.GetStreamVersion(ctx, namespace, streamName)
	if err != nil {
		t.Fatalf("failed to get stream version: %v", err)
	}

	if version != -1 {
		t.Errorf("expected version -1 for empty stream, got %d", version)
	}

	// Write messages
	for i := 0; i < 7; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "ItemAdded",
			Data:       map[string]interface{}{"item": i},
		}
		_, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}
	}

	// Get version after writes
	version, err = pgStore.GetStreamVersion(ctx, namespace, streamName)
	if err != nil {
		t.Fatalf("failed to get stream version: %v", err)
	}

	if version != 6 {
		t.Errorf("expected version 6, got %d", version)
	}
}
