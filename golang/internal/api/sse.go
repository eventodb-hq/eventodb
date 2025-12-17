package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/auth"
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
	testMode bool
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(st store.Store, testMode bool) *SSEHandler {
	return &SSEHandler{
		store:    st,
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

	// Send initial connection established event (optional)
	fmt.Fprintf(w, ": connected\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Start subscription loop
	if streamName != "" {
		h.subscribeToStream(ctx, w, namespace, streamName, position)
	} else {
		h.subscribeToCategory(ctx, w, namespace, categoryName, position, consumerMember, consumerSize)
	}
}

// subscribeToStream handles stream-specific subscriptions
func (h *SSEHandler) subscribeToStream(ctx context.Context, w http.ResponseWriter, namespace, streamName string, startPosition int64) {
	ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500ms
	defer ticker.Stop()

	lastPosition := startPosition

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Check for new messages
			messages, err := h.store.GetStreamMessages(ctx, namespace, streamName, &store.GetOpts{
				Position:  lastPosition,
				BatchSize: 100, // Check up to 100 new messages
			})
			if err != nil {
				log.Printf("Error fetching stream messages for subscription: %v", err)
				continue
			}

			// Send pokes for each new message
			for _, msg := range messages {
				poke := Poke{
					Stream:         streamName,
					Position:       msg.Position,
					GlobalPosition: msg.GlobalPosition,
				}
				if err := h.sendPoke(w, poke); err != nil {
					return // Connection closed
				}
				lastPosition = msg.Position + 1
			}
		}
	}
}

// subscribeToCategory handles category-specific subscriptions
func (h *SSEHandler) subscribeToCategory(ctx context.Context, w http.ResponseWriter, namespace, categoryName string, startPosition int64, consumerMember, consumerSize int64) {
	ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500ms
	defer ticker.Stop()

	lastGlobalPosition := startPosition

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Build options for category query
			opts := &store.CategoryOpts{
				Position:  lastGlobalPosition,
				BatchSize: 100, // Check up to 100 new messages
			}
			if consumerSize > 0 {
				opts.ConsumerMember = &consumerMember
				opts.ConsumerSize = &consumerSize
			}

			// Check for new messages
			messages, err := h.store.GetCategoryMessages(ctx, namespace, categoryName, opts)
			if err != nil {
				log.Printf("Error fetching category messages for subscription: %v", err)
				continue
			}

			// Send pokes for each new message
			for _, msg := range messages {
				poke := Poke{
					Stream:         msg.StreamName,
					Position:       msg.Position,
					GlobalPosition: msg.GlobalPosition,
				}
				if err := h.sendPoke(w, poke); err != nil {
					return // Connection closed
				}
				lastGlobalPosition = msg.GlobalPosition + 1
			}
		}
	}
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
	// In test mode, auth is optional
	if h.testMode {
		// Try to get namespace from token, but don't require it
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// No auth header, use default namespace
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
		// Check query parameter as fallback
		if token := r.URL.Query().Get("token"); token != "" {
			namespace, err := auth.ParseToken(token)
			if err != nil {
				return "", fmt.Errorf("invalid token in query parameter")
			}
			// Validate namespace exists in database
			ctx := r.Context()
			_, err = h.store.GetNamespace(ctx, namespace)
			if err != nil {
				return "", fmt.Errorf("unauthorized: namespace not found")
			}
			return namespace, nil
		}
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
