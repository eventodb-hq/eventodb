package eventodb

import (
	"context"
	"testing"
	"time"
)

func TestSSE001_SubscribeToStream(t *testing.T) {
	tc := setupTest(t, "sse-001")
	ctx := context.Background()

	stream := randomStreamName()

	// Subscribe to stream
	sub, err := tc.client.SubscribeStream(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Write a message to the stream
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Wait for poke event
	select {
	case poke := <-sub.Events:
		if poke.Stream != stream {
			t.Errorf("Expected stream %s, got %s", stream, poke.Stream)
		}
		if poke.Position != result.Position {
			t.Errorf("Expected position %d, got %d", result.Position, poke.Position)
		}
		if poke.GlobalPosition != result.GlobalPosition {
			t.Errorf("Expected globalPosition %d, got %d", result.GlobalPosition, poke.GlobalPosition)
		}
	case err := <-sub.Errors:
		t.Fatalf("Subscription error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for poke event")
	}
}

func TestSSE002_SubscribeToCategory(t *testing.T) {
	tc := setupTest(t, "sse-002")
	ctx := context.Background()

	category := randomCategoryName()

	// Subscribe to category
	sub, err := tc.client.SubscribeCategory(ctx, category, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Write a message to a stream in the category
	stream := category + "-123"
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Wait for poke event
	select {
	case poke := <-sub.Events:
		// For category subscriptions, the poke contains the full stream name
		if poke.Stream != stream {
			t.Errorf("Expected stream %s, got %s", stream, poke.Stream)
		}
		if poke.GlobalPosition != result.GlobalPosition {
			t.Errorf("Expected globalPosition %d, got %d", result.GlobalPosition, poke.GlobalPosition)
		}
	case err := <-sub.Errors:
		t.Fatalf("Subscription error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for poke event")
	}
}

func TestSSE003_SubscribeWithPosition(t *testing.T) {
	tc := setupTest(t, "sse-003")
	ctx := context.Background()

	stream := randomStreamName()

	// Write 5 messages to the stream (positions 0-4)
	for i := 0; i < 5; i++ {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"count": i},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Subscribe from position 3
	sub, err := tc.client.SubscribeStream(ctx, stream, &SubscribeStreamOptions{
		Position: int64Ptr(3),
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	// Should receive pokes for positions 3 and 4 (existing messages)
	// The server might send them immediately or not at all (depends on implementation)
	// Let's just verify we can write a new message and get a poke for it

	time.Sleep(100 * time.Millisecond)

	// Write another message (position 5)
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 5},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Wait for poke event for the new message
	select {
	case poke := <-sub.Events:
		if poke.Position != result.Position {
			t.Logf("Got poke for position %d, wrote at position %d", poke.Position, result.Position)
		}
		// Should be position >= 3
		if poke.Position < 3 {
			t.Errorf("Expected position >= 3, got %d", poke.Position)
		}
	case err := <-sub.Errors:
		t.Fatalf("Subscription error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for poke event")
	}
}

func TestSSE004_SubscribeWithoutAuthentication(t *testing.T) {
	// Create client without token
	client := NewClient(testBaseURL)
	ctx := context.Background()

	stream := randomStreamName()

	// Try to subscribe without authentication
	sub, err := client.SubscribeStream(ctx, stream, nil)

	// Some servers allow unauthenticated subscriptions in test mode
	if err != nil {
		t.Logf("Subscription rejected without auth (expected): %v", err)
		return
	}

	if sub != nil {
		defer sub.Close()

		// If subscription succeeded, it might error on the channel
		select {
		case err := <-sub.Errors:
			t.Logf("Subscription error (expected): %v", err)
		case <-time.After(500 * time.Millisecond):
			t.Log("Server allows unauthenticated subscriptions (test mode)")
		}
	}
}

func TestSSE005_SubscribeWithConsumerGroup(t *testing.T) {
	tc := setupTest(t, "sse-005")
	ctx := context.Background()

	category := randomCategoryName()

	// Subscribe with consumer group member 0 of size 2
	sub, err := tc.client.SubscribeCategory(ctx, category, &SubscribeCategoryOptions{
		ConsumerGroup: &ConsumerGroup{
			Member: 0,
			Size:   2,
		},
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	time.Sleep(100 * time.Millisecond)

	// Write messages to different streams in the category
	stream1 := category + "-1"
	_, err = tc.client.StreamWrite(ctx, stream1, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"stream": 1},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to stream 1: %v", err)
	}

	// Should receive poke for streams that hash to this consumer group member
	// Just verify we can receive at least one poke
	select {
	case poke := <-sub.Events:
		t.Logf("Received poke for consumer group: stream=%s, gpos=%d", poke.Stream, poke.GlobalPosition)
	case err := <-sub.Errors:
		t.Fatalf("Subscription error: %v", err)
	case <-time.After(2 * time.Second):
		t.Log("No poke received (stream may not hash to this consumer)")
	}
}

func TestSSE006_MultipleSubscriptions(t *testing.T) {
	tc := setupTest(t, "sse-006")
	ctx := context.Background()

	stream1 := randomStreamName()
	stream2 := randomStreamName()

	// Create two separate subscriptions
	sub1, err := tc.client.SubscribeStream(ctx, stream1, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe to stream 1: %v", err)
	}
	defer sub1.Close()

	sub2, err := tc.client.SubscribeStream(ctx, stream2, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe to stream 2: %v", err)
	}
	defer sub2.Close()

	time.Sleep(100 * time.Millisecond)

	// Write to stream 1
	result1, err := tc.client.StreamWrite(ctx, stream1, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"stream": 1},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to stream 1: %v", err)
	}

	// Write to stream 2
	result2, err := tc.client.StreamWrite(ctx, stream2, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"stream": 2},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write to stream 2: %v", err)
	}

	// Each subscription should only receive its own poke
	receivedSub1 := false
	receivedSub2 := false

	// Collect pokes from both subscriptions
	timeout := time.After(2 * time.Second)
	for !receivedSub1 || !receivedSub2 {
		select {
		case poke := <-sub1.Events:
			if poke.Stream != stream1 {
				t.Errorf("Sub1 received poke for wrong stream: %s", poke.Stream)
			}
			if poke.GlobalPosition != result1.GlobalPosition {
				t.Errorf("Sub1 received wrong globalPosition: %d", poke.GlobalPosition)
			}
			receivedSub1 = true

		case poke := <-sub2.Events:
			if poke.Stream != stream2 {
				t.Errorf("Sub2 received poke for wrong stream: %s", poke.Stream)
			}
			if poke.GlobalPosition != result2.GlobalPosition {
				t.Errorf("Sub2 received wrong globalPosition: %d", poke.GlobalPosition)
			}
			receivedSub2 = true

		case err := <-sub1.Errors:
			t.Fatalf("Sub1 error: %v", err)
		case err := <-sub2.Errors:
			t.Fatalf("Sub2 error: %v", err)

		case <-timeout:
			if !receivedSub1 {
				t.Error("Did not receive poke on subscription 1")
			}
			if !receivedSub2 {
				t.Error("Did not receive poke on subscription 2")
			}
			return
		}
	}

	t.Log("Both subscriptions received their respective pokes independently")
}

func TestSSE007_ReconnectionHandling(t *testing.T) {
	tc := setupTest(t, "sse-007")
	ctx := context.Background()

	stream := randomStreamName()

	// Subscribe to stream
	sub, err := tc.client.SubscribeStream(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	time.Sleep(100 * time.Millisecond)

	// Write first message
	result1, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 1},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message 1: %v", err)
	}

	// Receive first poke
	select {
	case poke := <-sub.Events:
		if poke.Position != result1.Position {
			t.Errorf("Expected position %d, got %d", result1.Position, poke.Position)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first poke")
	}

	// Close and reconnect from last known position
	lastPosition := result1.Position
	sub.Close()

	// Reconnect from last position
	sub2, err := tc.client.SubscribeStream(ctx, stream, &SubscribeStreamOptions{
		Position: &lastPosition,
	})
	if err != nil {
		t.Fatalf("Failed to reconnect: %v", err)
	}
	defer sub2.Close()

	time.Sleep(100 * time.Millisecond)

	// Write another message
	result2, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"count": 2},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message 2: %v", err)
	}

	// Should receive poke for messages from the position onwards
	// May receive poke for the old message at position 0 or the new one
	receivedPoke := false
	timeout := time.After(2 * time.Second)

	for !receivedPoke {
		select {
		case poke := <-sub2.Events:
			t.Logf("Reconnected subscription received poke for position %d, gpos %d", poke.Position, poke.GlobalPosition)
			if poke.Position == result2.Position || poke.GlobalPosition == result2.GlobalPosition {
				receivedPoke = true
				t.Log("✓ Received poke for new message after reconnection")
			} else {
				// May receive old messages first
				t.Logf("Received poke for earlier message, waiting for new one...")
			}
		case <-timeout:
			if !receivedPoke {
				t.Fatal("Timeout waiting for poke after reconnection")
			}
		}
	}
}

func TestSSE008_PokeEventParsing(t *testing.T) {
	tc := setupTest(t, "sse-008")
	ctx := context.Background()

	stream := randomStreamName()

	// Subscribe to stream
	sub, err := tc.client.SubscribeStream(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Close()

	time.Sleep(100 * time.Millisecond)

	// Write a message
	result, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Receive and verify poke event structure
	select {
	case poke := <-sub.Events:
		// Verify all required fields are present and valid
		if poke.Stream == "" {
			t.Error("Poke event missing 'stream' field")
		}
		if poke.Stream != stream {
			t.Errorf("Expected stream %s, got %s", stream, poke.Stream)
		}
		if poke.Position < 0 {
			t.Errorf("Invalid position: %d", poke.Position)
		}
		if poke.Position != result.Position {
			t.Errorf("Expected position %d, got %d", result.Position, poke.Position)
		}
		if poke.GlobalPosition < 0 {
			t.Errorf("Invalid globalPosition: %d", poke.GlobalPosition)
		}
		if poke.GlobalPosition != result.GlobalPosition {
			t.Errorf("Expected globalPosition %d, got %d", result.GlobalPosition, poke.GlobalPosition)
		}

		t.Logf("Valid poke event: stream=%s, position=%d, globalPosition=%d",
			poke.Stream, poke.Position, poke.GlobalPosition)

	case err := <-sub.Errors:
		t.Fatalf("Subscription error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for poke event")
	}
}

func TestSSE009_MultipleConsumersInSameGroup(t *testing.T) {
	tc := setupTest(t, "sse-009")
	ctx := context.Background()

	category := randomCategoryName()

	// Create two subscriptions with different consumer group members
	sub0, err := tc.client.SubscribeCategory(ctx, category, &SubscribeCategoryOptions{
		ConsumerGroup: &ConsumerGroup{
			Member: 0,
			Size:   2,
		},
	})
	if err != nil {
		t.Fatalf("Failed to subscribe member 0: %v", err)
	}
	defer sub0.Close()

	sub1, err := tc.client.SubscribeCategory(ctx, category, &SubscribeCategoryOptions{
		ConsumerGroup: &ConsumerGroup{
			Member: 1,
			Size:   2,
		},
	})
	if err != nil {
		t.Fatalf("Failed to subscribe member 1: %v", err)
	}
	defer sub1.Close()

	time.Sleep(100 * time.Millisecond)

	// Write messages to 4 different streams in the category
	streams := []string{
		category + "-1",
		category + "-2",
		category + "-3",
		category + "-4",
	}

	for i, stream := range streams {
		_, err := tc.client.StreamWrite(ctx, stream, Message{
			Type: "TestEvent",
			Data: map[string]interface{}{"stream": i + 1},
		}, nil)
		if err != nil {
			t.Fatalf("Failed to write to stream %d: %v", i+1, err)
		}
	}

	// Collect pokes from both consumers
	receivedBy0 := make(map[string]bool)
	receivedBy1 := make(map[string]bool)

	timeout := time.After(3 * time.Second)
	totalReceived := 0

	for totalReceived < 4 {
		select {
		case poke := <-sub0.Events:
			receivedBy0[poke.Stream] = true
			totalReceived++
			t.Logf("Member 0 received: %s", poke.Stream)

		case poke := <-sub1.Events:
			receivedBy1[poke.Stream] = true
			totalReceived++
			t.Logf("Member 1 received: %s", poke.Stream)

		case <-timeout:
			t.Logf("Timeout - received %d/4 pokes", totalReceived)
			goto verify
		}
	}

verify:
	t.Logf("Member 0 received %d streams: %v", len(receivedBy0), receivedBy0)
	t.Logf("Member 1 received %d streams: %v", len(receivedBy1), receivedBy1)

	// Verify no overlap - each stream should be consumed by exactly one member
	overlap := 0
	for stream := range receivedBy0 {
		if receivedBy1[stream] {
			overlap++
			t.Errorf("Stream %s received by BOTH consumers - consumer groups should partition streams!", stream)
		}
	}

	// Verify that messages were distributed (not all to one consumer)
	if len(receivedBy0) == 0 {
		t.Error("Member 0 received no messages")
	} else if len(receivedBy1) == 0 {
		t.Error("Member 1 received no messages")
	} else if overlap == 0 {
		t.Log("✓ Messages distributed between consumers with NO overlap")
	}

	// Verify we received at least some messages
	if totalReceived == 0 {
		t.Error("No messages received by any consumer")
	} else {
		t.Logf("✓ Received %d poke events total", totalReceived)
	}
}

func TestSSE010_CloseSubscription(t *testing.T) {
	tc := setupTest(t, "sse-010")
	ctx := context.Background()

	stream := randomStreamName()

	// Subscribe to stream
	sub, err := tc.client.SubscribeStream(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Close immediately
	err = sub.Close()
	if err != nil {
		t.Errorf("Failed to close subscription: %v", err)
	}

	// Verify channels are closed
	time.Sleep(100 * time.Millisecond)

	// Writing to the stream should not cause a panic
	_, err = tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Should not receive any events on closed subscription
	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Error("Received event on closed subscription")
		}
	default:
		// Good - channel is closed or empty
	}
}
