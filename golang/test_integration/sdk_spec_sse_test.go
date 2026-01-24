package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventodb/eventodb/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SSEClient manages a Server-Sent Events connection
type SSEClient struct {
	conn    *http.Response
	scanner *bufio.Scanner
	events  chan map[string]interface{}
	errors  chan error
	done    chan bool
	ready   chan bool
	mu      sync.Mutex
	running bool
}

// NewSSEClient creates a new SSE client
func NewSSEClient(url string, token string) (*SSEClient, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{
		Timeout: 0, // No timeout for SSE
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("SSE connection failed with status: %d", resp.StatusCode)
	}

	sseClient := &SSEClient{
		conn:    resp,
		scanner: bufio.NewScanner(resp.Body),
		events:  make(chan map[string]interface{}, 100),
		errors:  make(chan error, 10),
		done:    make(chan bool),
		ready:   make(chan bool, 1),
		running: true,
	}

	go sseClient.readEvents()

	return sseClient, nil
}

// readEvents reads SSE events from the connection
func (c *SSEClient) readEvents() {
	defer close(c.events)
	defer close(c.errors)
	defer close(c.ready)

	var eventData strings.Builder
	readySent := false

	for c.scanner.Scan() {
		line := c.scanner.Text()

		// Check for ready signal (comment line)
		if strings.HasPrefix(line, ":") {
			if strings.Contains(line, "ready") && !readySent {
				c.ready <- true
				readySent = true
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			eventData.WriteString(strings.TrimPrefix(line, "data: "))
		} else if line == "" && eventData.Len() > 0 {
			// End of event
			data := eventData.String()
			eventData.Reset()

			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				c.errors <- fmt.Errorf("failed to parse event: %w", err)
				continue
			}

			c.events <- event
		}
	}

	if err := c.scanner.Err(); err != nil {
		c.mu.Lock()
		if c.running {
			c.errors <- err
		}
		c.mu.Unlock()
	}
}

// WaitForReady waits for the subscription to be ready
func (c *SSEClient) WaitForReady(timeout time.Duration) error {
	select {
	case <-c.ready:
		return nil
	case err := <-c.errors:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for ready signal")
	}
}

// WaitForEvent waits for an event with timeout
func (c *SSEClient) WaitForEvent(timeout time.Duration) (map[string]interface{}, error) {
	select {
	case event := <-c.events:
		return event, nil
	case err := <-c.errors:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for event")
	}
}

// Close closes the SSE connection
func (c *SSEClient) Close() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	if c.conn != nil {
		c.conn.Body.Close()
	}
}

// TestSSE001_SubscribeToStream validates subscribing to a stream
func TestSSE001_SubscribeToStream(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("sse-test")

	// Subscribe to stream
	subscribeURL := fmt.Sprintf("%s/subscribe?stream=%s&token=%s", ts.URL(), stream, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write a message to the stream
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	expectedPosition := int64(resultMap["position"].(float64))
	expectedGlobalPosition := int64(resultMap["globalPosition"].(float64))

	// Wait for poke event
	event, err := client.WaitForEvent(2 * time.Second)
	require.NoError(t, err, "Should receive poke event")

	// Verify poke event structure
	assert.Equal(t, stream, event["stream"])
	assert.Equal(t, float64(expectedPosition), event["position"])
	assert.Equal(t, float64(expectedGlobalPosition), event["globalPosition"])
}

// TestSSE002_SubscribeToCategory validates subscribing to a category
func TestSSE002_SubscribeToCategory(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Use a simple category name without random suffix
	// The category is the part before the first dash in the stream name
	category := fmt.Sprintf("ssetest%d", time.Now().UnixNano()%1000000)
	stream := category + "-123"

	// Subscribe to category
	subscribeURL := fmt.Sprintf("%s/subscribe?category=%s&token=%s", ts.URL(), category, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write a message to a stream in the category
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	expectedGlobalPosition := int64(resultMap["globalPosition"].(float64))

	// Wait for poke event with longer timeout
	event, err := client.WaitForEvent(3 * time.Second)
	require.NoError(t, err, "Should receive poke event")

	// Verify poke event structure for category
	// Note: Category subscriptions still use "stream" field (the specific stream name, not category)
	assert.Contains(t, event["stream"].(string), category, "Stream name should contain category")
	assert.Equal(t, float64(expectedGlobalPosition), event["globalPosition"])
}

// TestSSE003_SubscribeWithPosition validates subscribing from a specific position
func TestSSE003_SubscribeWithPosition(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("sse-test")

	// Write 5 messages first
	for i := 0; i < 5; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Subscribe from position 3
	subscribeURL := fmt.Sprintf("%s/subscribe?stream=%s&position=3&token=%s", ts.URL(), stream, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Should receive pokes for existing positions 3, 4 immediately after ready
	// Drain existing events (positions 3 and 4)
	drainedCount := 0
	for i := 0; i < 2; i++ {
		_, err := client.WaitForEvent(500 * time.Millisecond)
		if err == nil {
			drainedCount++
		} else {
			break
		}
	}
	// We should have received at least the existing messages
	require.GreaterOrEqual(t, drainedCount, 2, "Should receive pokes for existing messages")

	// Write a new message
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"seq": 5},
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})
	expectedPosition := int64(resultMap["position"].(float64))

	// Wait for poke event for the new message
	// We might receive multiple pokes (for existing messages from position 3 onwards)
	// So we need to read events until we get the one for our new message
	var event map[string]interface{}
	found := false
	for i := 0; i < 5; i++ { // Try up to 5 events
		evt, err := client.WaitForEvent(2 * time.Second)
		if err != nil {
			break
		}
		event = evt
		if int64(event["position"].(float64)) == expectedPosition {
			found = true
			break
		}
	}

	require.True(t, found, "Should receive poke event for new message at position %d", expectedPosition)
	assert.Equal(t, stream, event["stream"])
}

// TestSSE004_SubscribeWithoutAuthentication validates subscription fails without auth
func TestSSE004_SubscribeWithoutAuthentication(t *testing.T) {
	// Create a test server in production mode (not test mode)
	env := SetupTestEnv(t)
	defer env.Cleanup()

	// Create pubsub for real-time notifications
	pubsub := api.NewPubSub()

	// Create SSE handler in production mode (testMode = false)
	sseHandler := api.NewSSEHandler(env.Store, pubsub, false)

	// Set up HTTP route
	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", sseHandler.HandleSubscribe)

	// Start server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}

	go server.Serve(listener)
	defer server.Close()
	defer listener.Close()

	time.Sleep(50 * time.Millisecond) // Give server time to start

	stream := randomStreamName("sse-test")

	// Try to subscribe without token
	subscribeURL := fmt.Sprintf("http://127.0.0.1:%d/subscribe?stream=%s", port, stream)
	resp, err := http.Get(subscribeURL)

	// Should fail with authentication error
	if err == nil {
		defer resp.Body.Close()
		require.NotEqual(t, http.StatusOK, resp.StatusCode, "Should fail without authentication")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 Unauthorized")
	} else {
		// Connection might be rejected immediately, which is also acceptable
		t.Logf("Connection rejected: %v", err)
	}
}

// TestSSE005_SubscribeWithConsumerGroup validates subscription with consumer group
func TestSSE005_SubscribeWithConsumerGroup(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Use a simple category name without random suffix
	category := fmt.Sprintf("ssetest%d", time.Now().UnixNano()%1000000)

	// Subscribe with consumer group (member 0 of 2)
	// Note: Parameters are "consumer" and "size", not "consumerGroup.member" and "consumerGroup.size"
	subscribeURL := fmt.Sprintf("%s/subscribe?category=%s&consumer=0&size=2&token=%s",
		ts.URL(), category, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write messages to multiple streams
	for i := 1; i <= 4; i++ {
		stream := fmt.Sprintf("%s-%d", category, i)
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"stream": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Should receive pokes only for streams assigned to member 0
	// We'll just verify we receive at least one poke with longer timeout
	event, err := client.WaitForEvent(3 * time.Second)
	require.NoError(t, err, "Should receive at least one poke for member 0's partition")
	// Verify the stream contains the category prefix
	assert.Contains(t, event["stream"].(string), category)
}

// TestSSE006_MultipleSubscriptions validates multiple independent subscriptions
func TestSSE006_MultipleSubscriptions(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream1 := randomStreamName("sse-test-1")
	stream2 := randomStreamName("sse-test-2")

	// Create two subscriptions
	subscribeURL1 := fmt.Sprintf("%s/subscribe?stream=%s&token=%s", ts.URL(), stream1, ts.Token)
	client1, err := NewSSEClient(subscribeURL1, ts.Token)
	require.NoError(t, err)
	defer client1.Close()

	subscribeURL2 := fmt.Sprintf("%s/subscribe?stream=%s&token=%s", ts.URL(), stream2, ts.Token)
	client2, err := NewSSEClient(subscribeURL2, ts.Token)
	require.NoError(t, err)
	defer client2.Close()

	// Wait for both subscriptions to be ready
	err = client1.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription 1 should be ready")
	err = client2.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription 2 should be ready")

	// Write to stream1
	msg1 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"stream": 1},
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream1, msg1)
	require.NoError(t, err)

	// Write to stream2
	msg2 := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"stream": 2},
	}
	_, err = makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream2, msg2)
	require.NoError(t, err)

	// Client1 should receive poke for stream1
	event1, err := client1.WaitForEvent(2 * time.Second)
	require.NoError(t, err)
	assert.Equal(t, stream1, event1["stream"])

	// Client2 should receive poke for stream2
	event2, err := client2.WaitForEvent(2 * time.Second)
	require.NoError(t, err)
	assert.Equal(t, stream2, event2["stream"])
}

// TestSSE007_ReconnectionHandling validates reconnection handling
func TestSSE007_ReconnectionHandling(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("sse-test")

	// Subscribe to stream
	subscribeURL := fmt.Sprintf("%s/subscribe?stream=%s&token=%s", ts.URL(), stream, ts.Token)
	client1, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)

	// Wait for subscription to be ready
	err = client1.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write some messages
	for i := 0; i < 3; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Read all events from first connection
	var lastPosition int64
	for i := 0; i < 3; i++ {
		event, err := client1.WaitForEvent(2 * time.Second)
		require.NoError(t, err, "Should receive event %d", i)
		lastPosition = int64(event["position"].(float64))
	}

	// Close first connection (simulating disconnect)
	client1.Close()

	// Write more messages while disconnected
	for i := 3; i < 6; i++ {
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
		require.NoError(t, err)
	}

	// Reconnect from last known stream position + 1
	reconnectURL := fmt.Sprintf("%s/subscribe?stream=%s&position=%d&token=%s",
		ts.URL(), stream, lastPosition+1, ts.Token)
	client2, err := NewSSEClient(reconnectURL, ts.Token)
	require.NoError(t, err)
	defer client2.Close()

	// Wait for subscription to be ready
	err = client2.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Reconnected subscription should be ready")

	// Should receive the messages that were written while disconnected
	// Note: We're using stream position, not global position for the subscription
	// So we should get messages from the stream position, not global position
	receivedCount := 0
	for i := 0; i < 5; i++ { // Try to read up to 5 events
		_, err := client2.WaitForEvent(500 * time.Millisecond)
		if err != nil {
			break
		}
		receivedCount++
	}

	// We should have received at least the 3 messages written while disconnected
	// (might receive more if position calculation includes earlier messages)
	require.GreaterOrEqual(t, receivedCount, 3,
		"Should receive messages written during disconnect")
}

// TestSSE008_PokeEventParsing validates poke event structure
func TestSSE008_PokeEventParsing(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	stream := randomStreamName("sse-test")

	// Subscribe to stream
	subscribeURL := fmt.Sprintf("%s/subscribe?stream=%s&token=%s", ts.URL(), stream, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write a message
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
	require.NoError(t, err)

	resultMap := result.(map[string]interface{})

	// Wait for poke event
	event, err := client.WaitForEvent(2 * time.Second)
	require.NoError(t, err)

	// Verify poke data is valid JSON with required fields
	assert.NotNil(t, event, "Poke should be valid JSON object")

	// For stream subscription, should have: stream, position, globalPosition
	assert.Contains(t, event, "stream", "Poke should contain 'stream' field")
	assert.Contains(t, event, "position", "Poke should contain 'position' field")
	assert.Contains(t, event, "globalPosition", "Poke should contain 'globalPosition' field")

	// Verify values match what we wrote
	assert.Equal(t, stream, event["stream"])
	assert.Equal(t, resultMap["position"], event["position"])
	assert.Equal(t, resultMap["globalPosition"], event["globalPosition"])

	// Verify types
	assert.IsType(t, "", event["stream"], "stream should be string")
	assert.IsType(t, float64(0), event["position"], "position should be number")
	assert.IsType(t, float64(0), event["globalPosition"], "globalPosition should be number")
}

// TestSSE009_MultipleConsumersInSameGroup validates multiple consumers in same consumer group
func TestSSE009_MultipleConsumersInSameGroup(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Use a unique category
	category := fmt.Sprintf("ssetest%d", time.Now().UnixNano()%1000000)

	// Create two consumers in the same consumer group (size 2)
	// Member 0
	subscribeURL0 := fmt.Sprintf("%s/subscribe?category=%s&consumer=0&size=2&token=%s",
		ts.URL(), category, ts.Token)
	client0, err := NewSSEClient(subscribeURL0, ts.Token)
	require.NoError(t, err)
	defer client0.Close()

	// Member 1
	subscribeURL1 := fmt.Sprintf("%s/subscribe?category=%s&consumer=1&size=2&token=%s",
		ts.URL(), category, ts.Token)
	client1, err := NewSSEClient(subscribeURL1, ts.Token)
	require.NoError(t, err)
	defer client1.Close()

	// Wait for both subscriptions to be ready
	err = client0.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Consumer 0 should be ready")
	err = client1.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Consumer 1 should be ready")

	// Write messages to 4 different streams in the category
	streams := make([]string, 4)
	for i := 0; i < 4; i++ {
		streams[i] = fmt.Sprintf("%s-%d", category, i+1)
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"streamId": i + 1},
		}
		_, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", streams[i], msg)
		require.NoError(t, err)
	}

	// Collect events from both consumers
	consumer0Streams := make(map[string]bool)
	consumer1Streams := make(map[string]bool)

	// Collect events from consumer 0 (with timeout)
	timeout := time.After(3 * time.Second)
	done0 := false
	for !done0 {
		select {
		case <-timeout:
			done0 = true
		default:
			event, err := client0.WaitForEvent(500 * time.Millisecond)
			if err != nil {
				done0 = true
			} else {
				streamName := event["stream"].(string)
				consumer0Streams[streamName] = true
			}
		}
	}

	// Collect events from consumer 1 (with timeout)
	timeout = time.After(3 * time.Second)
	done1 := false
	for !done1 {
		select {
		case <-timeout:
			done1 = true
		default:
			event, err := client1.WaitForEvent(500 * time.Millisecond)
			if err != nil {
				done1 = true
			} else {
				streamName := event["stream"].(string)
				consumer1Streams[streamName] = true
			}
		}
	}

	// Verify that:
	// 1. Both consumers received some events
	assert.Greater(t, len(consumer0Streams), 0, "Consumer 0 should receive some pokes")
	assert.Greater(t, len(consumer1Streams), 0, "Consumer 1 should receive some pokes")

	// 2. No stream is received by both consumers (exclusive partitioning)
	for stream := range consumer0Streams {
		assert.NotContains(t, consumer1Streams, stream,
			"Stream %s should not be received by both consumers", stream)
	}

	for stream := range consumer1Streams {
		assert.NotContains(t, consumer0Streams, stream,
			"Stream %s should not be received by both consumers", stream)
	}

	// 3. All streams are covered by at least one consumer
	allStreamsReceived := make(map[string]bool)
	for stream := range consumer0Streams {
		allStreamsReceived[stream] = true
	}
	for stream := range consumer1Streams {
		allStreamsReceived[stream] = true
	}

	for _, stream := range streams {
		assert.Contains(t, allStreamsReceived, stream,
			"Stream %s should be received by at least one consumer", stream)
	}

	t.Logf("Consumer 0 received pokes from streams: %v", getKeys(consumer0Streams))
	t.Logf("Consumer 1 received pokes from streams: %v", getKeys(consumer1Streams))
}

// Helper function to get map keys as a slice
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestSSE010_SubscribeToAll validates subscribing to all events in a namespace
func TestSSE010_SubscribeToAll(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Subscribe to all events using ?all=true
	subscribeURL := fmt.Sprintf("%s/subscribe?all=true&token=%s", ts.URL(), ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err, "Should be able to subscribe with all=true")
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write messages to different categories
	stream1 := randomStreamName("category1")
	stream2 := randomStreamName("category2")
	stream3 := randomStreamName("category3")

	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"foo": "bar"},
	}

	// Write to stream1
	result1, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream1, msg)
	require.NoError(t, err)
	result1Map := result1.(map[string]interface{})

	// Write to stream2
	result2, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream2, msg)
	require.NoError(t, err)
	result2Map := result2.(map[string]interface{})

	// Write to stream3
	result3, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream3, msg)
	require.NoError(t, err)
	result3Map := result3.(map[string]interface{})

	// Collect all received pokes
	receivedStreams := make(map[string]int64)
	for i := 0; i < 3; i++ {
		event, err := client.WaitForEvent(2 * time.Second)
		require.NoError(t, err, "Should receive poke event %d", i+1)
		streamName := event["stream"].(string)
		globalPos := int64(event["globalPosition"].(float64))
		receivedStreams[streamName] = globalPos
	}

	// Verify we received pokes for all three streams
	assert.Contains(t, receivedStreams, stream1, "Should receive poke for stream1")
	assert.Contains(t, receivedStreams, stream2, "Should receive poke for stream2")
	assert.Contains(t, receivedStreams, stream3, "Should receive poke for stream3")

	// Verify global positions match
	assert.Equal(t, int64(result1Map["globalPosition"].(float64)), receivedStreams[stream1])
	assert.Equal(t, int64(result2Map["globalPosition"].(float64)), receivedStreams[stream2])
	assert.Equal(t, int64(result3Map["globalPosition"].(float64)), receivedStreams[stream3])
}

// TestSSE011_SubscribeToAllWithPosition validates subscribing to all from a specific position
func TestSSE011_SubscribeToAllWithPosition(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Write some messages first
	streams := make([]string, 3)
	globalPositions := make([]int64, 3)
	for i := 0; i < 3; i++ {
		streams[i] = randomStreamName(fmt.Sprintf("cat%d", i))
		msg := map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{"seq": i},
		}
		result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", streams[i], msg)
		require.NoError(t, err)
		globalPositions[i] = int64(result.(map[string]interface{})["globalPosition"].(float64))
	}

	// Subscribe from position after the first message
	startPosition := globalPositions[0] + 1
	subscribeURL := fmt.Sprintf("%s/subscribe?all=true&position=%d&token=%s",
		ts.URL(), startPosition, ts.Token)
	client, err := NewSSEClient(subscribeURL, ts.Token)
	require.NoError(t, err)
	defer client.Close()

	// Wait for subscription to be ready
	err = client.WaitForReady(2 * time.Second)
	require.NoError(t, err, "Subscription should be ready")

	// Write a new message
	newStream := randomStreamName("newcat")
	msg := map[string]interface{}{
		"type": "TestEvent",
		"data": map[string]interface{}{"new": true},
	}
	result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", newStream, msg)
	require.NoError(t, err)
	newGlobalPos := int64(result.(map[string]interface{})["globalPosition"].(float64))

	// Should receive poke for the new message
	event, err := client.WaitForEvent(2 * time.Second)
	require.NoError(t, err, "Should receive poke for new message")
	assert.Equal(t, newStream, event["stream"])
	assert.Equal(t, float64(newGlobalPos), event["globalPosition"])
}

// TestSSE012_SubscribeToAllCannotCombineWithStreamOrCategory validates error handling
func TestSSE012_SubscribeToAllCannotCombineWithStreamOrCategory(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Try to combine all=true with stream - should fail
	subscribeURL := fmt.Sprintf("%s/subscribe?all=true&stream=test&token=%s", ts.URL(), ts.Token)
	_, err := NewSSEClient(subscribeURL, ts.Token)
	require.Error(t, err, "Should fail when combining all=true with stream")

	// Try to combine all=true with category - should fail
	subscribeURL = fmt.Sprintf("%s/subscribe?all=true&category=test&token=%s", ts.URL(), ts.Token)
	_, err = NewSSEClient(subscribeURL, ts.Token)
	require.Error(t, err, "Should fail when combining all=true with category")
}
