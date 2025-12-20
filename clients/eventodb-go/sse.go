package eventodb

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PokeEvent represents a notification that a new message is available
type PokeEvent struct {
	Stream         string `json:"stream"`         // Full stream name (for both stream and category subscriptions)
	Position       int64  `json:"position"`       // Position in the stream
	GlobalPosition int64  `json:"globalPosition"` // Global position across all streams
}

// Subscription represents an active SSE subscription
type Subscription struct {
	Events chan PokeEvent
	Errors chan error
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	closed bool
}

// Close closes the subscription and releases resources
func (s *Subscription) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Cancel the context to stop the goroutine
	s.cancel()

	// Wait for goroutine to finish (with timeout)
	select {
	case <-s.done:
	case <-time.After(1 * time.Second):
		// Timeout waiting for cleanup
	}

	return nil
}

// SubscribeStreamOptions configures stream subscription
type SubscribeStreamOptions struct {
	Position *int64 `json:"position,omitempty"`
}

// SubscribeCategoryOptions configures category subscription
type SubscribeCategoryOptions struct {
	Position      *int64         `json:"position,omitempty"`
	ConsumerGroup *ConsumerGroup `json:"consumerGroup,omitempty"`
}

// SubscribeStream subscribes to poke events for a stream
func (c *Client) SubscribeStream(ctx context.Context, streamName string, opts *SubscribeStreamOptions) (*Subscription, error) {
	if opts == nil {
		opts = &SubscribeStreamOptions{}
	}

	params := map[string]string{
		"stream": streamName,
	}

	if opts.Position != nil {
		params["position"] = strconv.FormatInt(*opts.Position, 10)
	}

	return c.subscribe(ctx, "/subscribe", params)
}

// SubscribeCategory subscribes to poke events for a category
func (c *Client) SubscribeCategory(ctx context.Context, categoryName string, opts *SubscribeCategoryOptions) (*Subscription, error) {
	if opts == nil {
		opts = &SubscribeCategoryOptions{}
	}

	params := map[string]string{
		"category": categoryName,
	}

	if opts.Position != nil {
		params["position"] = strconv.FormatInt(*opts.Position, 10)
	}

	if opts.ConsumerGroup != nil {
		params["consumer"] = strconv.Itoa(opts.ConsumerGroup.Member)
		params["size"] = strconv.Itoa(opts.ConsumerGroup.Size)
	}

	return c.subscribe(ctx, "/subscribe", params)
}

func (c *Client) subscribe(ctx context.Context, path string, params map[string]string) (*Subscription, error) {
	// Build URL with query parameters
	url := c.baseURL + path
	if len(params) > 0 {
		var parts []string
		for k, v := range params {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		url += "?" + strings.Join(parts, "&")
	}

	// Create subscription context (independent of request context)
	subCtx, cancel := context.WithCancel(context.Background())

	// Create request with subscription context
	req, err := http.NewRequestWithContext(subCtx, "GET", url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	sub := &Subscription{
		Events: make(chan PokeEvent, 10),
		Errors: make(chan error, 10),
		ctx:    subCtx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// Start subscription in background
	go func() {
		defer func() {
			close(sub.done)
			close(sub.Events)
			close(sub.Errors)
		}()

		// Make request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			select {
			case sub.Errors <- fmt.Errorf("request failed: %w", err):
			case <-subCtx.Done():
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			select {
			case sub.Errors <- fmt.Errorf("HTTP %d: subscription failed", resp.StatusCode):
			case <-subCtx.Done():
			}
			return
		}

		// Read SSE stream
		reader := bufio.NewReader(resp.Body)
		var eventData string

		for {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case sub.Errors <- fmt.Errorf("read error: %w", err):
					case <-subCtx.Done():
					}
				}
				return
			}

			line = strings.TrimSpace(line)

			// Empty line indicates end of event
			if line == "" {
				if eventData != "" {
					// Parse and send event
					var poke PokeEvent
					if err := json.Unmarshal([]byte(eventData), &poke); err != nil {
						select {
						case sub.Errors <- fmt.Errorf("failed to parse event: %w", err):
						case <-subCtx.Done():
						}
					} else {
						select {
						case sub.Events <- poke:
						case <-subCtx.Done():
							return
						}
					}
					eventData = ""
				}
				continue
			}

			// Parse SSE line
			if strings.HasPrefix(line, "data: ") {
				eventData = strings.TrimPrefix(line, "data: ")
			}
			// Ignore other SSE fields (event:, id:, retry:, comment)
		}
	}()

	return sub, nil
}
