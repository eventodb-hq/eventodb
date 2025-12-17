package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"

	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/sqlite"
)

// Example_basicUsage demonstrates basic message writing and reading
func Example_basicUsage() {
	ctx := context.Background()

	// Setup in-memory SQLite store for example
	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	// Create namespace
	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write a message
	msg := &store.Message{
		StreamName: "account-123",
		Type:       "AccountCreated",
		Data: map[string]interface{}{
			"accountId": "123",
			"name":      "John Doe",
			"email":     "john@example.com",
		},
	}

	result, _ := st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
	fmt.Printf("Wrote message at position %d\n", result.Position)

	// Read messages from stream
	opts := &store.GetOpts{
		Position:  0,
		BatchSize: 10,
	}

	messages, _ := st.GetStreamMessages(ctx, "myapp", "account-123", opts)
	for _, m := range messages {
		fmt.Printf("Message type: %s\n", m.Type)
	}

	// Output:
	// Wrote message at position 0
	// Message type: AccountCreated
}

// Example_optimisticLocking demonstrates version-based concurrency control
func Example_optimisticLocking() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write initial message
	msg1 := &store.Message{
		StreamName: "account-123",
		Type:       "AccountCreated",
		Data:       map[string]interface{}{"balance": 0},
	}
	st.WriteMessage(ctx, "myapp", msg1.StreamName, msg1)

	// Get current version
	version, _ := st.GetStreamVersion(ctx, "myapp", "account-123")
	fmt.Printf("Current version: %d\n", version)

	// Write with expected version (will succeed)
	msg2 := &store.Message{
		StreamName:      "account-123",
		Type:            "AccountCredited",
		Data:            map[string]interface{}{"amount": 100},
		ExpectedVersion: &version,
	}

	_, err := st.WriteMessage(ctx, "myapp", msg2.StreamName, msg2)
	if err == nil {
		fmt.Println("Write succeeded with correct version")
	}

	// Try to write with old version (will fail)
	msg3 := &store.Message{
		StreamName:      "account-123",
		Type:            "AccountCredited",
		Data:            map[string]interface{}{"amount": 50},
		ExpectedVersion: &version, // Old version!
	}

	_, err = st.WriteMessage(ctx, "myapp", msg3.StreamName, msg3)
	if err != nil {
		fmt.Println("Write failed with stale version")
	}

	// Output:
	// Current version: 0
	// Write succeeded with correct version
	// Write failed with stale version
}

// Example_categoryQueries demonstrates reading from multiple streams
func Example_categoryQueries() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write messages to multiple account streams
	for i := 1; i <= 3; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("account-%d", i),
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"accountId": i,
			},
		}
		st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
	}

	// Read all messages from the "account" category
	opts := &store.CategoryOpts{
		Position:  1,
		BatchSize: 100,
	}

	messages, _ := st.GetCategoryMessages(ctx, "myapp", "account", opts)
	fmt.Printf("Found %d messages in category\n", len(messages))

	for _, m := range messages {
		fmt.Printf("Stream: %s, Type: %s\n", m.StreamName, m.Type)
	}

	// Output:
	// Found 3 messages in category
	// Stream: account-1, Type: AccountCreated
	// Stream: account-2, Type: AccountCreated
	// Stream: account-3, Type: AccountCreated
}

// Example_consumerGroups demonstrates parallel processing with consumer groups
func Example_consumerGroups() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write messages to multiple streams
	for i := 1; i <= 10; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("account-%d", i),
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"accountId": i,
			},
		}
		st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
	}

	// Consumer 0 of 2-member group
	consumer0 := int64(0)
	groupSize := int64(2)

	opts := &store.CategoryOpts{
		Position:       1,
		BatchSize:      100,
		ConsumerMember: &consumer0,
		ConsumerSize:   &groupSize,
	}

	messages, _ := st.GetCategoryMessages(ctx, "myapp", "account", opts)
	fmt.Printf("Consumer 0 handles %d messages\n", len(messages))

	// Consumer 1 of 2-member group
	consumer1 := int64(1)
	opts.ConsumerMember = &consumer1

	messages, _ = st.GetCategoryMessages(ctx, "myapp", "account", opts)
	fmt.Printf("Consumer 1 handles %d messages\n", len(messages))

	// Output will vary based on hash function, but shows partitioning
	// Consumer 0 handles 5 messages
	// Consumer 1 handles 5 messages
}

// Example_utilityFunctions demonstrates stream name parsing
func Example_utilityFunctions() {
	// Setup store (any backend)
	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	// Parse simple stream name
	streamName := "account-123"
	fmt.Printf("Category: %s\n", st.Category(streamName))
	fmt.Printf("ID: %s\n", st.ID(streamName))
	fmt.Printf("CardinalID: %s\n", st.CardinalID(streamName))
	fmt.Printf("IsCategory: %t\n", st.IsCategory(streamName))

	// Parse compound stream name
	compoundStream := "account-123+deposit"
	fmt.Printf("\nCompound stream: %s\n", compoundStream)
	fmt.Printf("Category: %s\n", st.Category(compoundStream))
	fmt.Printf("ID: %s\n", st.ID(compoundStream))
	fmt.Printf("CardinalID: %s\n", st.CardinalID(compoundStream))

	// Category name (no ID)
	categoryName := "account"
	fmt.Printf("\nCategory name: %s\n", categoryName)
	fmt.Printf("IsCategory: %t\n", st.IsCategory(categoryName))
	fmt.Printf("ID: %s\n", st.ID(categoryName))

	// Output:
	// Category: account
	// ID: 123
	// CardinalID: 123
	// IsCategory: false
	//
	// Compound stream: account-123+deposit
	// Category: account
	// ID: 123+deposit
	// CardinalID: 123
	//
	// Category name: account
	// IsCategory: true
	// ID:
}

// Example_namespaceIsolation demonstrates physical data separation
func Example_namespaceIsolation() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	// Create two separate namespaces
	_ = st.CreateNamespace(ctx, "tenant-a", "hash_a", "Tenant A")
	_ = st.CreateNamespace(ctx, "tenant-b", "hash_b", "Tenant B")

	// Write to same stream name in different namespaces
	msg := &store.Message{
		StreamName: "account-1",
		Type:       "AccountCreated",
		Data:       map[string]interface{}{"tenant": "A"},
	}
	st.WriteMessage(ctx, "tenant-a", msg.StreamName, msg)

	msg.Data = map[string]interface{}{"tenant": "B"}
	st.WriteMessage(ctx, "tenant-b", msg.StreamName, msg)

	// Read from tenant-a
	opts := &store.GetOpts{Position: 0, BatchSize: 10}
	messagesA, _ := st.GetStreamMessages(ctx, "tenant-a", "account-1", opts)
	fmt.Printf("Tenant A messages: %d\n", len(messagesA))
	fmt.Printf("Tenant A data: %v\n", messagesA[0].Data["tenant"])

	// Read from tenant-b
	messagesB, _ := st.GetStreamMessages(ctx, "tenant-b", "account-1", opts)
	fmt.Printf("Tenant B messages: %d\n", len(messagesB))
	fmt.Printf("Tenant B data: %v\n", messagesB[0].Data["tenant"])

	// Output:
	// Tenant A messages: 1
	// Tenant A data: A
	// Tenant B messages: 1
	// Tenant B data: B
}

// Example_eventSourcing demonstrates a typical event sourcing pattern
func Example_eventSourcing() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	accountID := "123"
	streamName := fmt.Sprintf("account-%s", accountID)

	// Event 1: Account created
	msg1 := &store.Message{
		StreamName: streamName,
		Type:       "AccountCreated",
		Data: map[string]interface{}{
			"accountId": accountID,
			"name":      "John Doe",
			"balance":   0,
		},
	}
	st.WriteMessage(ctx, "myapp", streamName, msg1)

	// Event 2: Money deposited
	msg2 := &store.Message{
		StreamName: streamName,
		Type:       "AccountCredited",
		Data: map[string]interface{}{
			"amount": 100,
		},
	}
	st.WriteMessage(ctx, "myapp", streamName, msg2)

	// Event 3: Money withdrawn
	msg3 := &store.Message{
		StreamName: streamName,
		Type:       "AccountDebited",
		Data: map[string]interface{}{
			"amount": 30,
		},
	}
	st.WriteMessage(ctx, "myapp", streamName, msg3)

	// Replay events to rebuild state
	opts := &store.GetOpts{Position: 0, BatchSize: 100}
	events, _ := st.GetStreamMessages(ctx, "myapp", streamName, opts)

	var balance float64
	var accountName string

	for _, event := range events {
		switch event.Type {
		case "AccountCreated":
			accountName = event.Data["name"].(string)
			balance = event.Data["balance"].(float64)
		case "AccountCredited":
			balance += event.Data["amount"].(float64)
		case "AccountDebited":
			balance -= event.Data["amount"].(float64)
		}
	}

	fmt.Printf("Account: %s\n", accountName)
	fmt.Printf("Balance: %.0f\n", balance)
	fmt.Printf("Events processed: %d\n", len(events))

	// Output:
	// Account: John Doe
	// Balance: 70
	// Events processed: 3
}

// Example_correlationFiltering demonstrates correlation-based queries
func Example_correlationFiltering() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write payment events with correlation to orders
	for i := 1; i <= 5; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("payment-%d", i),
			Type:       "PaymentProcessed",
			Data: map[string]interface{}{
				"amount": i * 100,
			},
			Metadata: map[string]interface{}{
				"correlationStreamName": fmt.Sprintf("order-%d", (i%2)+1),
			},
		}
		st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
	}

	// Query payments correlated with order category (order-1 and order-2)
	correlation := "order"
	opts := &store.CategoryOpts{
		Position:    1,
		BatchSize:   100,
		Correlation: &correlation,
	}

	messages, _ := st.GetCategoryMessages(ctx, "myapp", "payment", opts)
	fmt.Printf("Payments for order category: %d\n", len(messages))

	for _, m := range messages {
		fmt.Printf("Payment amount: %.0f\n", m.Data["amount"])
	}

	// Output:
	// Payments for order category: 5
	// Payment amount: 100
	// Payment amount: 200
	// Payment amount: 300
	// Payment amount: 400
	// Payment amount: 500
}

// Example_errorHandling demonstrates proper error handling
func Example_errorHandling() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// 1. Write to non-existent namespace
	msg := &store.Message{
		StreamName: "account-123",
		Type:       "Test",
		Data:       map[string]interface{}{},
	}

	_, err := st.WriteMessage(ctx, "nonexistent", msg.StreamName, msg)
	if err != nil {
		fmt.Println("Error: Namespace not found")
	}

	// 2. Version conflict
	st.WriteMessage(ctx, "myapp", "account-123", msg)

	wrongVersion := int64(999)
	msg.ExpectedVersion = &wrongVersion
	_, err = st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
	if err != nil {
		fmt.Println("Error: Version conflict")
	}

	// 3. Stream not found
	_, err = st.GetLastStreamMessage(ctx, "myapp", "nonexistent-stream", nil)
	if err != nil {
		fmt.Println("Error: Stream not found")
	}

	// Output:
	// Error: Namespace not found
	// Error: Version conflict
	// Error: Stream not found
}

// Example_lastMessage demonstrates retrieving the last message
func Example_lastMessage() {
	ctx := context.Background()

	db, _ := sql.Open("sqlite", ":memory:")
	st, _ := sqlite.New(db, &sqlite.Config{TestMode: true})
	defer st.Close()

	_ = st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")

	// Write several messages
	streamName := "account-123"
	for i := 1; i <= 5; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       fmt.Sprintf("Event%d", i),
			Data:       map[string]interface{}{"sequence": i},
		}
		st.WriteMessage(ctx, "myapp", streamName, msg)
	}

	// Get last message (any type)
	lastMsg, _ := st.GetLastStreamMessage(ctx, "myapp", streamName, nil)
	fmt.Printf("Last message type: %s\n", lastMsg.Type)

	// Get last message of specific type
	msgType := "Event3"
	lastOfType, err := st.GetLastStreamMessage(ctx, "myapp", streamName, &msgType)
	if err == nil {
		fmt.Printf("Last Event3 type: %s, position: %d\n", lastOfType.Type, lastOfType.Position)
	}

	// Output:
	// Last message type: Event5
	// Last Event3 type: Event3, position: 2
}

func init() {
	// Suppress log output in examples
	log.SetOutput(nil)
}
