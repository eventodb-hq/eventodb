// Package api provides fasthttp-native SSE handler
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/eventodb/eventodb/internal/logger"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/valyala/fasthttp"
)

// FastHTTPSSEHandler wraps the SSE handler with fasthttp streaming support
func FastHTTPSSEHandler(h *SSEHandler, testMode bool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Get namespace from context (set by auth middleware)
		namespace, ok := GetNamespaceFromFastHTTP(ctx)
		if !ok {
			namespace = "default"
		}

		// Parse query parameters
		args := ctx.QueryArgs()
		streamName := string(args.Peek("stream"))
		categoryName := string(args.Peek("category"))
		subscribeAll := string(args.Peek("all")) == "true"

		// Validate: need exactly one of stream, category, or all
		if !subscribeAll && streamName == "" && categoryName == "" {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
			ctx.SetBodyString("Either 'stream', 'category', or 'all=true' parameter required")
			return
		}
		if subscribeAll && (streamName != "" || categoryName != "") {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
			ctx.SetBodyString("Cannot combine 'all' with 'stream' or 'category'")
			return
		}
		if streamName != "" && categoryName != "" {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
			ctx.SetBodyString("Cannot specify both 'stream' and 'category'")
			return
		}

		// Parse position parameter
		position := int64(0)
		if posStr := string(args.Peek("position")); posStr != "" {
			pos, err := strconv.ParseInt(posStr, 10, 64)
			if err != nil {
				ctx.SetStatusCode(fasthttp.StatusBadRequest)
				ctx.SetBodyString("Invalid position parameter")
				return
			}
			position = pos
		}

		// Parse consumer group parameters (for category subscriptions)
		var consumerMember, consumerSize int64
		if categoryName != "" {
			if memberStr := string(args.Peek("consumer")); memberStr != "" {
				member, err := strconv.ParseInt(memberStr, 10, 64)
				if err != nil {
					ctx.SetStatusCode(fasthttp.StatusBadRequest)
					ctx.SetBodyString("Invalid consumer parameter")
					return
				}
				consumerMember = member
			}
			if sizeStr := string(args.Peek("size")); sizeStr != "" {
				size, err := strconv.ParseInt(sizeStr, 10, 64)
				if err != nil {
					ctx.SetStatusCode(fasthttp.StatusBadRequest)
					ctx.SetBodyString("Invalid size parameter")
					return
				}
				consumerSize = size
			}
			// Validate consumer group parameters
			if (consumerMember > 0 || consumerSize > 0) && (consumerSize == 0 || consumerMember >= consumerSize) {
				ctx.SetStatusCode(fasthttp.StatusBadRequest)
				ctx.SetBodyString("Invalid consumer group: member must be < size")
				return
			}
		}

		// Set SSE headers
		ctx.SetContentType("text/event-stream")
		ctx.Response.Header.Set("Cache-Control", "no-cache")
		ctx.Response.Header.Set("Connection", "keep-alive")
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")

		// Use SetBodyStreamWriter for streaming response
		ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
			// Start subscription based on type
			if subscribeAll {
				handleAllSubscriptionFast(w, h, namespace, position)
			} else if streamName != "" {
				handleStreamSubscriptionFast(w, h, namespace, streamName, position)
			} else {
				handleCategorySubscriptionFast(w, h, namespace, categoryName, position, consumerMember, consumerSize)
			}
		})
	}
}

// handleStreamSubscriptionFast handles stream-specific subscriptions for fasthttp
func handleStreamSubscriptionFast(w *bufio.Writer, h *SSEHandler, namespace, streamName string, startPosition int64) {
	// First, send any existing messages from startPosition
	messages, err := h.Store.GetStreamMessages(context.Background(), namespace, streamName, &store.GetOpts{
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
		poke := pokePool.Get().(*Poke)
		poke.Stream = streamName
		poke.Position = msg.Position
		poke.GlobalPosition = msg.GlobalPosition

		err := sendPokeFast(w, poke)
		pokePool.Put(poke)

		if err != nil {
			return
		}
		lastPosition = msg.Position + 1
	}

	// Subscribe to real-time updates (if pubsub is available)
	if h.Pubsub == nil {
		return
	}

	sub := h.Pubsub.SubscribeStream(namespace, streamName)
	defer h.Pubsub.UnsubscribeStream(namespace, streamName, sub)

	for event := range sub {
		// Only send if position >= our tracking position
		if event.Position >= lastPosition {
			poke := pokePool.Get().(*Poke)
			poke.Stream = event.Stream
			poke.Position = event.Position
			poke.GlobalPosition = event.GlobalPosition

			err := sendPokeFast(w, poke)
			pokePool.Put(poke)

			if err != nil {
				return
			}
			lastPosition = event.Position + 1
		}
	}
}

// handleCategorySubscriptionFast handles category subscriptions for fasthttp
func handleCategorySubscriptionFast(w *bufio.Writer, h *SSEHandler, namespace, categoryName string, startPosition, consumerMember, consumerSize int64) {
	// First, send any existing messages from startPosition
	opts := &store.CategoryOpts{
		Position:  startPosition,
		BatchSize: 1000,
	}
	if consumerSize > 0 {
		opts.ConsumerSize = &consumerSize
		opts.ConsumerMember = &consumerMember
	}

	messages, err := h.Store.GetCategoryMessages(context.Background(), namespace, categoryName, opts)
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
		poke := pokePool.Get().(*Poke)
		poke.Stream = msg.StreamName
		poke.Position = msg.Position
		poke.GlobalPosition = msg.GlobalPosition

		err := sendPokeFast(w, poke)
		pokePool.Put(poke)

		if err != nil {
			return
		}
		lastGlobalPosition = msg.GlobalPosition + 1
	}

	// Subscribe to real-time updates (if pubsub is available)
	if h.Pubsub == nil {
		return
	}

	sub := h.Pubsub.SubscribeCategory(namespace, categoryName)
	defer h.Pubsub.UnsubscribeCategory(namespace, categoryName, sub)

	for event := range sub {
		// Only send if globalPosition >= our tracking position
		if event.GlobalPosition >= lastGlobalPosition {
			// Apply consumer group filter if needed
			if consumerSize > 0 && !matchesConsumerGroup(event.Stream, consumerMember, consumerSize) {
				continue
			}
			poke := pokePool.Get().(*Poke)
			poke.Stream = event.Stream
			poke.Position = event.Position
			poke.GlobalPosition = event.GlobalPosition

			err := sendPokeFast(w, poke)
			pokePool.Put(poke)

			if err != nil {
				return
			}
			lastGlobalPosition = event.GlobalPosition + 1
		}
	}
}

// sendPokeFast sends a poke event via SSE using fasthttp buffered writer
func sendPokeFast(w *bufio.Writer, poke *Poke) error {
	data, err := json.Marshal(poke)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: poke\ndata: %s\n\n", data)
	if err != nil {
		return err
	}

	// Flush to send the event immediately
	return w.Flush()
}

// matchesConsumerGroup checks if a stream belongs to a consumer group member
func matchesConsumerGroup(streamName string, member, size int64) bool {
	// Hash the stream name to determine which consumer it belongs to
	hash := uint64(0)
	for _, c := range streamName {
		hash = hash*31 + uint64(c)
	}
	return int64(hash%uint64(size)) == member
}

// handleAllSubscriptionFast handles namespace-wide subscriptions for fasthttp
func handleAllSubscriptionFast(w *bufio.Writer, h *SSEHandler, namespace string, startPosition int64) {
	// Send ready signal
	fmt.Fprintf(w, ": ready\n\n")
	w.Flush()

	// Subscribe to real-time updates (if pubsub is available)
	if h.Pubsub == nil {
		return
	}

	sub := h.Pubsub.SubscribeAll(namespace)
	defer h.Pubsub.UnsubscribeAll(namespace, sub)

	for event := range sub {
		// Only send if globalPosition >= startPosition
		if event.GlobalPosition >= startPosition {
			poke := pokePool.Get().(*Poke)
			poke.Stream = event.Stream
			poke.Position = event.Position
			poke.GlobalPosition = event.GlobalPosition

			err := sendPokeFast(w, poke)
			pokePool.Put(poke)

			if err != nil {
				return
			}
		}
	}
}
