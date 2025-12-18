package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/logger"
	"github.com/message-db/message-db/internal/store"
)

// Poke represents a lightweight notification sent via SSE
type Poke struct {
	Stream         string `json:"stream"`
	Position       int64  `json:"position"`
	GlobalPosition int64  `json:"globalPosition"`
}

// SSEHandler manages Server-Sent Events subscriptions
type SSEHandler struct {
	store    store.Store
	pubsub   *PubSub
	testMode bool
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(st store.Store, pubsub *PubSub, testMode bool) *SSEHandler {
	return &SSEHandler{
		store:    st,
		pubsub:   pubsub,
		testMode: testMode,
	}
}

// HandleSubscribe handles SSE subscription requests
// Supports both stream and category subscriptions
func (h *SSEHandler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract and validate token
	namespace, err := h.extractNamespace(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	streamName := query.Get("stream")
	categoryName := query.Get("category")

	// Validate that either stream or category is specified, but not both
	if streamName == "" && categoryName == "" {
		http.Error(w, "Either 'stream' or 'category' parameter required", http.StatusBadRequest)
		return
	}
	if streamName != "" && categoryName != "" {
		http.Error(w, "Cannot specify both 'stream' and 'category'", http.StatusBadRequest)
		return
	}

	// Parse position parameter
	position := int64(0)
	if posStr := query.Get("position"); posStr != "" {
		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid position parameter", http.StatusBadRequest)
			return
		}
		position = pos
	}

	// Parse consumer group parameters (for category subscriptions)
	var consumerMember, consumerSize int64
	if categoryName != "" {
		if memberStr := query.Get("consumer"); memberStr != "" {
			member, err := strconv.ParseInt(memberStr, 10, 64)
			if err != nil {
				http.Error(w, "Invalid consumer parameter", http.StatusBadRequest)
				return
			}
			consumerMember = member
		}
		if sizeStr := query.Get("size"); sizeStr != "" {
			size, err := strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				http.Error(w, "Invalid size parameter", http.StatusBadRequest)
				return
			}
			consumerSize = size
		}
		// Validate consumer group parameters
		if (consumerMember > 0 || consumerSize > 0) && (consumerSize == 0 || consumerMember >= consumerSize) {
			http.Error(w, "Invalid consumer group: member must be < size", http.StatusBadRequest)
			return
		}
	}

	// Get context for this request
	ctx := r.Context()

	// Flush headers immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Start subscription
	if streamName != "" {
		h.subscribeToStream(ctx, w, namespace, streamName, position)
	} else {
		h.subscribeToCategory(ctx, w, namespace, categoryName, position, consumerMember, consumerSize)
	}
}

// subscribeToStream handles stream-specific subscriptions
func (h *SSEHandler) subscribeToStream(ctx context.Context, w http.ResponseWriter, namespace, streamName string, startPosition int64) {
	// First, send any existing messages from startPosition
	messages, err := h.store.GetStreamMessages(ctx, namespace, streamName, &store.GetOpts{
		Position:  startPosition,
		BatchSize: 1000,
	})
	if err != nil {
		logger.Get().Error().
			Err(err).
			Str("stream", streamName).
			Str("namespace", namespace).
			Int64("position", startPosition).
			Msg("Error fetching initial stream messages")
	}

	lastPosition := startPosition
	for _, msg := range messages {
		poke := Poke{
			Stream:         streamName,
			Position:       msg.Position,
			GlobalPosition: msg.GlobalPosition,
		}
		if err := h.sendPoke(w, poke); err != nil {
			return
		}
		lastPosition = msg.Position + 1
	}

	// Subscribe to real-time updates (if pubsub is available)
	if h.pubsub == nil {
		// No pubsub, just wait for context cancellation
		<-ctx.Done()
		return
	}

	sub := h.pubsub.SubscribeStream(namespace, streamName)
	defer h.pubsub.UnsubscribeStream(namespace, streamName, sub)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub:
			if !ok {
				return
			}
			// Only send if position >= our tracking position
			if event.Position >= lastPosition {
				poke := Poke{
					Stream:         event.Stream,
					Position:       event.Position,
					GlobalPosition: event.GlobalPosition,
				}
				if err := h.sendPoke(w, poke); err != nil {
					return
				}
				lastPosition = event.Position + 1
			}
		}
	}
}

// subscribeToCategory handles category-specific subscriptions
func (h *SSEHandler) subscribeToCategory(ctx context.Context, w http.ResponseWriter, namespace, categoryName string, startPosition int64, consumerMember, consumerSize int64) {
	// Build options for category query
	opts := &store.CategoryOpts{
		Position:  startPosition,
		BatchSize: 1000,
	}
	if consumerSize > 0 {
		opts.ConsumerMember = &consumerMember
		opts.ConsumerSize = &consumerSize
	}

	// First, send any existing messages from startPosition
	messages, err := h.store.GetCategoryMessages(ctx, namespace, categoryName, opts)
	if err != nil {
		logger.Get().Error().
			Err(err).
			Str("category", categoryName).
			Str("namespace", namespace).
			Int64("position", startPosition).
			Msg("Error fetching initial category messages")
	}

	lastGlobalPosition := startPosition
	for _, msg := range messages {
		// Apply consumer group filter if needed
		if consumerSize > 0 && !h.matchesConsumerGroup(msg.StreamName, consumerMember, consumerSize) {
			continue
		}
		poke := Poke{
			Stream:         msg.StreamName,
			Position:       msg.Position,
			GlobalPosition: msg.GlobalPosition,
		}
		if err := h.sendPoke(w, poke); err != nil {
			return
		}
		lastGlobalPosition = msg.GlobalPosition + 1
	}

	// Subscribe to real-time updates (if pubsub is available)
	if h.pubsub == nil {
		// No pubsub, just wait for context cancellation
		<-ctx.Done()
		return
	}

	sub := h.pubsub.SubscribeCategory(namespace, categoryName)
	defer h.pubsub.UnsubscribeCategory(namespace, categoryName, sub)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub:
			if !ok {
				return
			}
			// Only send if globalPosition >= our tracking position
			if event.GlobalPosition >= lastGlobalPosition {
				// Apply consumer group filter if needed
				if consumerSize > 0 && !h.matchesConsumerGroup(event.Stream, consumerMember, consumerSize) {
					continue
				}
				poke := Poke{
					Stream:         event.Stream,
					Position:       event.Position,
					GlobalPosition: event.GlobalPosition,
				}
				if err := h.sendPoke(w, poke); err != nil {
					return
				}
				lastGlobalPosition = event.GlobalPosition + 1
			}
		}
	}
}

// matchesConsumerGroup checks if a stream belongs to a consumer group member
func (h *SSEHandler) matchesConsumerGroup(streamName string, member, size int64) bool {
	// Hash the stream name to determine which consumer it belongs to
	hash := uint64(0)
	for _, c := range streamName {
		hash = hash*31 + uint64(c)
	}
	return int64(hash%uint64(size)) == member
}

// sendPoke sends a poke event via SSE
func (h *SSEHandler) sendPoke(w http.ResponseWriter, poke Poke) error {
	data, err := json.Marshal(poke)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: poke\ndata: %s\n\n", data)
	if err != nil {
		return err
	}

	// Flush the response writer to send the event immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// extractNamespace extracts and validates the namespace from the request
func (h *SSEHandler) extractNamespace(r *http.Request) (string, error) {
	// Check query parameter first (for SSE which can't set headers easily)
	if token := r.URL.Query().Get("token"); token != "" {
		namespace, err := auth.ParseToken(token)
		if err != nil {
			if h.testMode {
				return "default", nil
			}
			return "", fmt.Errorf("invalid token in query parameter")
		}
		if !h.testMode {
			// Validate namespace exists in database
			ctx := r.Context()
			_, err = h.store.GetNamespace(ctx, namespace)
			if err != nil {
				return "", fmt.Errorf("unauthorized: namespace not found")
			}
		}
		return namespace, nil
	}

	// In test mode, auth is optional
	if h.testMode {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return "default", nil
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return "default", nil
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		namespace, err := auth.ParseToken(token)
		if err != nil {
			return "default", nil
		}
		return namespace, nil
	}

	// In production mode, require authentication
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("authorization required")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	namespace, err := auth.ParseToken(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format")
	}

	// Validate namespace exists in database
	ctx := r.Context()
	_, err = h.store.GetNamespace(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("unauthorized: namespace not found")
	}

	return namespace, nil
}
