package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/postgres"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// backendFactory creates a store instance for testing
type backendFactory func(t *testing.T) (store.Store, func())

// getPostgresFactory returns a factory for creating Postgres stores
func getPostgresFactory() backendFactory {
	return func(t *testing.T) (store.Store, func()) {
		t.Helper()

		// Get connection info from environment or use defaults
		host := getEnv("POSTGRES_HOST", "localhost")
		port := getEnv("POSTGRES_PORT", "5432")
		user := getEnv("POSTGRES_USER", "postgres")
		password := getEnv("POSTGRES_PASSWORD", "postgres")
		dbname := getEnv("POSTGRES_DB", "postgres")

		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbname)

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			t.Skipf("Failed to connect to Postgres: %v (skipping Postgres tests)", err)
			return nil, nil
		}

		// Ping to verify connection
		if err := db.Ping(); err != nil {
			db.Close()
			t.Skipf("Failed to ping Postgres: %v (skipping Postgres tests)", err)
			return nil, nil
		}

		store, err := postgres.New(db)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to create PostgresStore: %v", err)
		}

		cleanup := func() {
			// Clean up test namespaces
			ctx := context.Background()
			namespaces, _ := store.ListNamespaces(ctx)
			for _, ns := range namespaces {
				if len(ns.ID) > 8 && (ns.ID[:8] == "test_ns_" || ns.ID[:8] == "test-ns-") {
					_ = store.DeleteNamespace(ctx, ns.ID)
				}
			}
			store.Close()
		}

		return store, cleanup
	}
}

// getSQLiteFactory returns a factory for creating SQLite stores
func getSQLiteFactory() backendFactory {
	return func(t *testing.T) (store.Store, func()) {
		t.Helper()

		// Use in-memory database for tests
		db, err := sql.Open("sqlite", "file::memory:?cache=shared")
		if err != nil {
			t.Fatalf("Failed to create in-memory database: %v", err)
		}

		if err := db.Ping(); err != nil {
			db.Close()
			t.Fatalf("Failed to ping database: %v", err)
		}

		config := &sqlite.Config{
			TestMode: true,
			DataDir:  t.TempDir(),
		}

		store, err := sqlite.New(db, config)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to create SQLiteStore: %v", err)
		}

		cleanup := func() {
			// Clean up test namespaces
			ctx := context.Background()
			namespaces, _ := store.ListNamespaces(ctx)
			for _, ns := range namespaces {
				_ = store.DeleteNamespace(ctx, ns.ID)
			}
			store.Close()
		}

		return store, cleanup
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// runWithBothBackends runs a test with both Postgres and SQLite backends
func runWithBothBackends(t *testing.T, testFunc func(t *testing.T, s store.Store)) {
	backends := map[string]backendFactory{
		"Postgres": getPostgresFactory(),
		"SQLite":   getSQLiteFactory(),
	}

	for name, factory := range backends {
		t.Run(name, func(t *testing.T) {
			s, cleanup := factory(t)
			if s == nil {
				return // Backend skipped
			}
			defer cleanup()

			testFunc(t, s)
		})
	}
}

// MDB001_6A_T1: Test namespace isolation (both backends)
func TestMDB001_6A_T1_NamespaceIsolation(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create two namespaces
		ns1 := fmt.Sprintf("test_ns_%d_a", time.Now().UnixNano())
		ns2 := fmt.Sprintf("test_ns_%d_b", time.Now().UnixNano())

		err := s.CreateNamespace(ctx, ns1, "token_hash_1", "Test namespace 1")
		if err != nil {
			t.Fatalf("Failed to create namespace 1: %v", err)
		}

		err = s.CreateNamespace(ctx, ns2, "token_hash_2", "Test namespace 2")
		if err != nil {
			t.Fatalf("Failed to create namespace 2: %v", err)
		}

		// Write message to namespace 1
		msg1 := &store.Message{
			StreamName: "account-123",
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"name": "John Doe",
			},
		}

		result1, err := s.WriteMessage(ctx, ns1, msg1.StreamName, msg1)
		if err != nil {
			t.Fatalf("Failed to write message to ns1: %v", err)
		}

		if result1.Position != 0 {
			t.Errorf("Expected position 0, got %d", result1.Position)
		}

		// Write message to namespace 2
		msg2 := &store.Message{
			StreamName: "account-123",
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"name": "Jane Smith",
			},
		}

		result2, err := s.WriteMessage(ctx, ns2, msg2.StreamName, msg2)
		if err != nil {
			t.Fatalf("Failed to write message to ns2: %v", err)
		}

		if result2.Position != 0 {
			t.Errorf("Expected position 0, got %d", result2.Position)
		}

		// Read from namespace 1 - should only see msg1
		messages1, err := s.GetStreamMessages(ctx, ns1, "account-123", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages from ns1: %v", err)
		}

		if len(messages1) != 1 {
			t.Fatalf("Expected 1 message in ns1, got %d", len(messages1))
		}

		if messages1[0].Data["name"] != "John Doe" {
			t.Errorf("Expected 'John Doe', got %v", messages1[0].Data["name"])
		}

		// Read from namespace 2 - should only see msg2
		messages2, err := s.GetStreamMessages(ctx, ns2, "account-123", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages from ns2: %v", err)
		}

		if len(messages2) != 1 {
			t.Fatalf("Expected 1 message in ns2, got %d", len(messages2))
		}

		if messages2[0].Data["name"] != "Jane Smith" {
			t.Errorf("Expected 'Jane Smith', got %v", messages2[0].Data["name"])
		}
	})
}

// MDB001_6A_T2: Test write/read parity (both backends)
func TestMDB001_6A_T2_WriteReadParity(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create namespace
		ns := fmt.Sprintf("test_ns_%d", time.Now().UnixNano())
		err := s.CreateNamespace(ctx, ns, "token_hash", "Test namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace: %v", err)
		}

		// Write multiple messages
		messages := []*store.Message{
			{
				StreamName: "account-456",
				Type:       "AccountCreated",
				Data:       map[string]interface{}{"balance": 100},
			},
			{
				StreamName: "account-456",
				Type:       "MoneyDeposited",
				Data:       map[string]interface{}{"amount": 50},
			},
			{
				StreamName: "account-456",
				Type:       "MoneyWithdrawn",
				Data:       map[string]interface{}{"amount": 25},
			},
		}

		for i, msg := range messages {
			result, err := s.WriteMessage(ctx, ns, msg.StreamName, msg)
			if err != nil {
				t.Fatalf("Failed to write message %d: %v", i, err)
			}

			if result.Position != int64(i) {
				t.Errorf("Expected position %d, got %d", i, result.Position)
			}
		}

		// Read all messages
		retrieved, err := s.GetStreamMessages(ctx, ns, "account-456", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		if len(retrieved) != len(messages) {
			t.Fatalf("Expected %d messages, got %d", len(messages), len(retrieved))
		}

		// Verify messages match
		for i, msg := range retrieved {
			if msg.Type != messages[i].Type {
				t.Errorf("Message %d: expected type %s, got %s", i, messages[i].Type, msg.Type)
			}
			if msg.Position != int64(i) {
				t.Errorf("Message %d: expected position %d, got %d", i, i, msg.Position)
			}
		}

		// Verify GetStreamVersion
		version, err := s.GetStreamVersion(ctx, ns, "account-456")
		if err != nil {
			t.Fatalf("Failed to get stream version: %v", err)
		}

		if version != 2 { // 0, 1, 2 = version 2 (last position)
			t.Errorf("Expected version 2, got %d", version)
		}

		// Verify GetLastStreamMessage
		last, err := s.GetLastStreamMessage(ctx, ns, "account-456", nil)
		if err != nil {
			t.Fatalf("Failed to get last message: %v", err)
		}

		if last.Type != "MoneyWithdrawn" {
			t.Errorf("Expected last message type 'MoneyWithdrawn', got %s", last.Type)
		}
	})
}

// MDB001_6A_T3: Test category queries parity
func TestMDB001_6A_T3_CategoryQueriesParity(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create namespace
		ns := fmt.Sprintf("test_ns_%d", time.Now().UnixNano())
		err := s.CreateNamespace(ctx, ns, "token_hash", "Test namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace: %v", err)
		}

		// Write messages to multiple streams in same category
		streams := []string{"account-111", "account-222", "account-333"}
		for _, stream := range streams {
			for i := 0; i < 3; i++ {
				msg := &store.Message{
					StreamName: stream,
					Type:       fmt.Sprintf("Event%d", i),
					Data:       map[string]interface{}{"stream": stream, "index": i},
				}
				_, err := s.WriteMessage(ctx, ns, msg.StreamName, msg)
				if err != nil {
					t.Fatalf("Failed to write message to %s: %v", stream, err)
				}
			}
		}

		// Query category
		opts := store.NewCategoryOpts()
		opts.BatchSize = 100
		messages, err := s.GetCategoryMessages(ctx, ns, "account", opts)
		if err != nil {
			t.Fatalf("Failed to get category messages: %v", err)
		}

		// Should have all 9 messages (3 streams × 3 messages)
		if len(messages) != 9 {
			t.Errorf("Expected 9 messages, got %d", len(messages))
		}

		// Verify messages are from the correct category
		for _, msg := range messages {
			category := s.Category(msg.StreamName)
			if category != "account" {
				t.Errorf("Expected category 'account', got %s", category)
			}
		}

		// Verify global positions are ordered
		for i := 1; i < len(messages); i++ {
			if messages[i].GlobalPosition <= messages[i-1].GlobalPosition {
				t.Errorf("Messages not ordered by global position: %d <= %d",
					messages[i].GlobalPosition, messages[i-1].GlobalPosition)
			}
		}
	})
}

// MDB001_6A_T4: Test consumer group partitioning parity
func TestMDB001_6A_T4_ConsumerGroupPartitioningParity(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create namespace
		ns := fmt.Sprintf("test_ns_%d", time.Now().UnixNano())
		err := s.CreateNamespace(ctx, ns, "token_hash", "Test namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace: %v", err)
		}

		// Write messages to multiple streams
		numStreams := 10
		for i := 0; i < numStreams; i++ {
			msg := &store.Message{
				StreamName: fmt.Sprintf("account-%d", i),
				Type:       "AccountCreated",
				Data:       map[string]interface{}{"id": i},
			}
			_, err := s.WriteMessage(ctx, ns, msg.StreamName, msg)
			if err != nil {
				t.Fatalf("Failed to write message: %v", err)
			}
		}

		// Query with consumer groups (2 consumers)
		consumerSize := int64(2)
		member0 := int64(0)
		member1 := int64(1)

		opts0 := store.NewCategoryOpts()
		opts0.ConsumerMember = &member0
		opts0.ConsumerSize = &consumerSize
		opts0.BatchSize = 100

		messages0, err := s.GetCategoryMessages(ctx, ns, "account", opts0)
		if err != nil {
			t.Fatalf("Failed to get messages for consumer 0: %v", err)
		}

		opts1 := store.NewCategoryOpts()
		opts1.ConsumerMember = &member1
		opts1.ConsumerSize = &consumerSize
		opts1.BatchSize = 100

		messages1, err := s.GetCategoryMessages(ctx, ns, "account", opts1)
		if err != nil {
			t.Fatalf("Failed to get messages for consumer 1: %v", err)
		}

		// Verify no overlap
		seenStreams := make(map[string]int)
		for _, msg := range messages0 {
			seenStreams[msg.StreamName] = 0
		}
		for _, msg := range messages1 {
			if member, exists := seenStreams[msg.StreamName]; exists {
				t.Errorf("Stream %s assigned to both consumers 0 and 1", msg.StreamName)
				t.Logf("Consumer %d got stream %s", member, msg.StreamName)
			}
			seenStreams[msg.StreamName] = 1
		}

		// Verify all messages accounted for
		totalMessages := len(messages0) + len(messages1)
		if totalMessages != numStreams {
			t.Errorf("Expected %d total messages, got %d", numStreams, totalMessages)
		}

		// Verify deterministic assignment (same hash should go to same consumer)
		for _, msg := range messages0 {
			cardinalID := s.CardinalID(msg.StreamName)
			hash := s.Hash64(cardinalID)
			if hash < 0 {
				hash = -hash
			}
			assignedMember := hash % consumerSize
			if assignedMember != member0 {
				t.Errorf("Stream %s: expected consumer %d, hash assigned to %d",
					msg.StreamName, member0, assignedMember)
			}
		}

		for _, msg := range messages1 {
			cardinalID := s.CardinalID(msg.StreamName)
			hash := s.Hash64(cardinalID)
			if hash < 0 {
				hash = -hash
			}
			assignedMember := hash % consumerSize
			if assignedMember != member1 {
				t.Errorf("Stream %s: expected consumer %d, hash assigned to %d",
					msg.StreamName, member1, assignedMember)
			}
		}
	})
}

// MDB001_6A_T5: Test optimistic locking parity
func TestMDB001_6A_T5_OptimisticLockingParity(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create namespace
		ns := fmt.Sprintf("test_ns_%d", time.Now().UnixNano())
		err := s.CreateNamespace(ctx, ns, "token_hash", "Test namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace: %v", err)
		}

		// Write initial message
		msg1 := &store.Message{
			StreamName: "account-789",
			Type:       "AccountCreated",
			Data:       map[string]interface{}{"balance": 0},
		}

		result1, err := s.WriteMessage(ctx, ns, msg1.StreamName, msg1)
		if err != nil {
			t.Fatalf("Failed to write message 1: %v", err)
		}

		if result1.Position != 0 {
			t.Errorf("Expected position 0, got %d", result1.Position)
		}

		// Write with correct expected version
		expectedVersion := int64(0)
		msg2 := &store.Message{
			StreamName:      "account-789",
			Type:            "MoneyDeposited",
			Data:            map[string]interface{}{"amount": 100},
			ExpectedVersion: &expectedVersion,
		}

		result2, err := s.WriteMessage(ctx, ns, msg2.StreamName, msg2)
		if err != nil {
			t.Fatalf("Failed to write message with correct version: %v", err)
		}

		if result2.Position != 1 {
			t.Errorf("Expected position 1, got %d", result2.Position)
		}

		// Write with incorrect expected version (should fail)
		wrongVersion := int64(0) // Current version is 1
		msg3 := &store.Message{
			StreamName:      "account-789",
			Type:            "MoneyWithdrawn",
			Data:            map[string]interface{}{"amount": 50},
			ExpectedVersion: &wrongVersion,
		}

		_, err = s.WriteMessage(ctx, ns, msg3.StreamName, msg3)
		if err == nil {
			t.Error("Expected version conflict error, got nil")
		}

		// Verify stream still at version 1
		messages, err := s.GetStreamMessages(ctx, ns, "account-789", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		if len(messages) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(messages))
		}
	})
}

// MDB001_6A_T6: Test concurrent writes to different namespaces
func TestMDB001_6A_T6_ConcurrentWritesDifferentNamespaces(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create two namespaces
		ns1 := fmt.Sprintf("test_ns_%d_x", time.Now().UnixNano())
		ns2 := fmt.Sprintf("test_ns_%d_y", time.Now().UnixNano())

		err := s.CreateNamespace(ctx, ns1, "token_hash_1", "Test namespace 1")
		if err != nil {
			t.Fatalf("Failed to create namespace 1: %v", err)
		}

		err = s.CreateNamespace(ctx, ns2, "token_hash_2", "Test namespace 2")
		if err != nil {
			t.Fatalf("Failed to create namespace 2: %v", err)
		}

		// Write concurrently to different namespaces
		done := make(chan error, 2)

		// Goroutine 1: Write to ns1
		go func() {
			for i := 0; i < 10; i++ {
				msg := &store.Message{
					StreamName: "test-stream",
					Type:       fmt.Sprintf("Event%d", i),
					Data:       map[string]interface{}{"index": i},
				}
				_, err := s.WriteMessage(ctx, ns1, msg.StreamName, msg)
				if err != nil {
					done <- fmt.Errorf("ns1 write %d failed: %w", i, err)
					return
				}
			}
			done <- nil
		}()

		// Goroutine 2: Write to ns2
		go func() {
			for i := 0; i < 10; i++ {
				msg := &store.Message{
					StreamName: "test-stream",
					Type:       fmt.Sprintf("Event%d", i),
					Data:       map[string]interface{}{"index": i},
				}
				_, err := s.WriteMessage(ctx, ns2, msg.StreamName, msg)
				if err != nil {
					done <- fmt.Errorf("ns2 write %d failed: %w", i, err)
					return
				}
			}
			done <- nil
		}()

		// Wait for both goroutines
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Fatalf("Concurrent write failed: %v", err)
			}
		}

		// Verify both namespaces have 10 messages
		messages1, err := s.GetStreamMessages(ctx, ns1, "test-stream", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages from ns1: %v", err)
		}

		if len(messages1) != 10 {
			t.Errorf("Expected 10 messages in ns1, got %d", len(messages1))
		}

		messages2, err := s.GetStreamMessages(ctx, ns2, "test-stream", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages from ns2: %v", err)
		}

		if len(messages2) != 10 {
			t.Errorf("Expected 10 messages in ns2, got %d", len(messages2))
		}
	})
}

// MDB001_6A_T7: Test concurrent writes to same stream
func TestMDB001_6A_T7_ConcurrentWritesSameStream(t *testing.T) {
	runWithBothBackends(t, func(t *testing.T, s store.Store) {
		ctx := context.Background()

		// Create namespace
		ns := fmt.Sprintf("test_ns_%d", time.Now().UnixNano())
		err := s.CreateNamespace(ctx, ns, "token_hash", "Test namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace: %v", err)
		}

		// Write concurrently to same stream
		numWriters := 5
		writesPerWriter := 4
		done := make(chan error, numWriters)

		for w := 0; w < numWriters; w++ {
			writer := w
			go func() {
				for i := 0; i < writesPerWriter; i++ {
					msg := &store.Message{
						StreamName: "concurrent-stream",
						Type:       fmt.Sprintf("Writer%dEvent%d", writer, i),
						Data:       map[string]interface{}{"writer": writer, "index": i},
					}
					_, err := s.WriteMessage(ctx, ns, msg.StreamName, msg)
					if err != nil {
						done <- fmt.Errorf("writer %d, write %d failed: %w", writer, i, err)
						return
					}
				}
				done <- nil
			}()
		}

		// Wait for all writers
		for i := 0; i < numWriters; i++ {
			if err := <-done; err != nil {
				t.Fatalf("Concurrent write failed: %v", err)
			}
		}

		// Verify all messages written
		messages, err := s.GetStreamMessages(ctx, ns, "concurrent-stream", store.NewGetOpts())
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		expectedCount := numWriters * writesPerWriter
		if len(messages) != expectedCount {
			t.Errorf("Expected %d messages, got %d", expectedCount, len(messages))
		}

		// Verify positions are sequential and gapless
		for i, msg := range messages {
			if msg.Position != int64(i) {
				t.Errorf("Position gap: expected %d, got %d", i, msg.Position)
			}
		}

		// Verify no duplicate positions
		seenPositions := make(map[int64]bool)
		for _, msg := range messages {
			if seenPositions[msg.Position] {
				t.Errorf("Duplicate position: %d", msg.Position)
			}
			seenPositions[msg.Position] = true
		}
	})
}
