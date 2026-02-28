// Package api provides RPC method handlers for stream operations.
package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eventodb/eventodb/internal/auth"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
)

// handleStreamWrite writes a message to a stream
// Request: ["stream.write", "streamName", {msg}, {opts}]
// Response: {"position": 6, "globalPosition": 1234}
func (h *RPCHandler) handleStreamWrite(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 2 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "stream.write requires at least 2 arguments: streamName and message",
		}
	}

	// Parse stream name
	streamName, ok := args[0].(string)
	if !ok || streamName == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "streamName must be a non-empty string",
		}
	}

	// Parse message object
	msgObj, ok := args[1].(map[string]interface{})
	if !ok {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "message must be an object",
		}
	}

	// Extract message type
	msgType, ok := msgObj["type"].(string)
	if !ok || msgType == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "message.type must be a non-empty string",
		}
	}

	// Extract message data
	data, ok := msgObj["data"].(map[string]interface{})
	if !ok {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "message.data must be an object",
		}
	}

	// Extract optional metadata
	var metadata map[string]interface{}
	if metaVal, exists := msgObj["metadata"]; exists {
		metadata, ok = metaVal.(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "message.metadata must be an object",
			}
		}
	}

	// Parse optional options
	var msgID string
	var expectedVersion *int64

	if len(args) > 2 {
		optsObj, ok := args[2].(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "options must be an object",
			}
		}

		// Extract optional ID
		if idVal, exists := optsObj["id"]; exists {
			msgID, ok = idVal.(string)
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.id must be a string",
				}
			}
		}

		// Extract optional expectedVersion
		if evVal, exists := optsObj["expectedVersion"]; exists {
			// Handle both float64 (from JSON) and int
			switch v := evVal.(type) {
			case float64:
				ev := int64(v)
				expectedVersion = &ev
			case int:
				ev := int64(v)
				expectedVersion = &ev
			case int64:
				expectedVersion = &v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.expectedVersion must be a number",
				}
			}
		}
	}

	// Generate ID if not provided
	if msgID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, &RPCError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("failed to generate UUID: %v", err),
			}
		}
		msgID = id.String()
	}

	// Create message
	msg := &store.Message{
		ID:              msgID,
		StreamName:      streamName,
		Type:            msgType,
		Data:            data,
		Metadata:        metadata,
		ExpectedVersion: expectedVersion,
	}

	// Get namespace from context
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	// Write message
	result, err := h.store.WriteMessage(ctx, namespace, streamName, msg)
	if err != nil {
		// Check for version conflict error
		if store.IsVersionConflict(err) {
			// Extract details from VersionConflictError if available
			if vcErr, ok := err.(*store.VersionConflictError); ok {
				return nil, &RPCError{
					Code:    "STREAM_VERSION_CONFLICT",
					Message: fmt.Sprintf("Expected version %d, stream is at version %d", vcErr.ExpectedVersion, vcErr.ActualVersion),
					Details: map[string]interface{}{
						"expected": vcErr.ExpectedVersion,
						"actual":   vcErr.ActualVersion,
					},
				}
			}
			// Fallback if we can't get details
			return nil, &RPCError{
				Code:    "STREAM_VERSION_CONFLICT",
				Message: err.Error(),
			}
		}

		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to write message: %v", err),
		}
	}

	// Publish event to subscribers (real-time notification)
	if h.pubsub != nil {
		category := store.Category(streamName)
		h.pubsub.Publish(WriteEvent{
			Namespace:      namespace,
			Stream:         streamName,
			Category:       category,
			Position:       result.Position,
			GlobalPosition: result.GlobalPosition,
		})
	}

	// Return result
	return map[string]interface{}{
		"position":       result.Position,
		"globalPosition": result.GlobalPosition,
	}, nil
}

// handleStreamGet retrieves messages from a stream
// Request: ["stream.get", "streamName", {opts}]
// Response: [[id, type, position, globalPosition, data, metadata, time], ...]
func (h *RPCHandler) handleStreamGet(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "stream.get requires at least 1 argument: streamName",
		}
	}

	// Parse stream name
	streamName, ok := args[0].(string)
	if !ok || streamName == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "streamName must be a non-empty string",
		}
	}

	// Parse options
	opts := store.NewGetOpts()

	if len(args) > 1 {
		optsObj, ok := args[1].(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "options must be an object",
			}
		}

		// Parse position
		if posVal, exists := optsObj["position"]; exists {
			switch v := posVal.(type) {
			case float64:
				opts.Position = int64(v)
			case int:
				opts.Position = int64(v)
			case int64:
				opts.Position = v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.position must be a number",
				}
			}
		}

		// Parse globalPosition (mutually exclusive with position)
		if gpVal, exists := optsObj["globalPosition"]; exists {
			switch v := gpVal.(type) {
			case float64:
				gp := int64(v)
				opts.GlobalPosition = &gp
			case int:
				gp := int64(v)
				opts.GlobalPosition = &gp
			case int64:
				opts.GlobalPosition = &v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.globalPosition must be a number",
				}
			}
		}

		// Parse batchSize
		if bsVal, exists := optsObj["batchSize"]; exists {
			switch v := bsVal.(type) {
			case float64:
				opts.BatchSize = int64(v)
			case int:
				opts.BatchSize = int64(v)
			case int64:
				opts.BatchSize = v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.batchSize must be a number",
				}
			}

			// Validate batch size
			if opts.BatchSize > 10000 && opts.BatchSize != -1 {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.batchSize must be <= 10000 or -1 for unlimited",
				}
			}
		}
	}

	// Get namespace from context
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	// Get messages
	messages, err := h.store.GetStreamMessages(ctx, namespace, streamName, opts)
	if err != nil {
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to get messages: %v", err),
		}
	}

	// Format response as array of arrays
	result := make([]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = []interface{}{
			msg.ID,
			msg.Type,
			msg.Position,
			msg.GlobalPosition,
			msg.Data,
			msg.Metadata,
			msg.Time.UTC().Format(time.RFC3339Nano),
		}
	}

	return result, nil
}

// handleStreamLast retrieves the last message from a stream
// Request: ["stream.last", "streamName", {opts}]
// Response: [id, type, position, globalPosition, data, metadata, time] or null
func (h *RPCHandler) handleStreamLast(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "stream.last requires at least 1 argument: streamName",
		}
	}

	// Parse stream name
	streamName, ok := args[0].(string)
	if !ok || streamName == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "streamName must be a non-empty string",
		}
	}

	// Parse optional type filter
	var msgType *string

	if len(args) > 1 {
		optsObj, ok := args[1].(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "options must be an object",
			}
		}

		if typeVal, exists := optsObj["type"]; exists {
			typeStr, ok := typeVal.(string)
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.type must be a string",
				}
			}
			msgType = &typeStr
		}
	}

	// Get namespace from context
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	// Get last message
	msg, err := h.store.GetLastStreamMessage(ctx, namespace, streamName, msgType)
	if err != nil {
		// If stream not found, return null
		if errors.Is(err, store.ErrStreamNotFound) {
			return nil, nil
		}
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to get last message: %v", err),
		}
	}

	// Return null if no message found
	if msg == nil {
		return nil, nil
	}

	// Format response as array
	return []interface{}{
		msg.ID,
		msg.Type,
		msg.Position,
		msg.GlobalPosition,
		msg.Data,
		msg.Metadata,
		msg.Time.UTC().Format(time.RFC3339Nano),
	}, nil
}

// handleStreamVersion returns the current version of a stream
// Request: ["stream.version", "streamName"]
// Response: 5 or null
func (h *RPCHandler) handleStreamVersion(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "stream.version requires 1 argument: streamName",
		}
	}

	// Parse stream name
	streamName, ok := args[0].(string)
	if !ok || streamName == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "streamName must be a non-empty string",
		}
	}

	// Get namespace from context
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	// Get stream version
	version, err := h.store.GetStreamVersion(ctx, namespace, streamName)
	if err != nil {
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to get stream version: %v", err),
		}
	}

	// Return null if stream doesn't exist (version is -1)
	if version == -1 {
		return nil, nil
	}

	return version, nil
}

// handleCategoryGet retrieves messages from all streams in a category
// Request: ["category.get", "categoryName", {opts}]
// Response: [[id, streamName, type, position, globalPosition, data, metadata, time], ...]
func (h *RPCHandler) handleCategoryGet(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "category.get requires at least 1 argument: categoryName",
		}
	}

	// Parse category name (empty string = all messages)
	categoryName, ok := args[0].(string)
	if !ok {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "categoryName must be a string",
		}
	}

	// Parse options
	opts := store.NewCategoryOpts()

	if len(args) > 1 {
		optsObj, ok := args[1].(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "options must be an object",
			}
		}

		// Parse position
		if posVal, exists := optsObj["position"]; exists {
			switch v := posVal.(type) {
			case float64:
				opts.Position = int64(v)
			case int:
				opts.Position = int64(v)
			case int64:
				opts.Position = v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.position must be a number",
				}
			}
		}

		// Parse globalPosition (alternative to position)
		if gpVal, exists := optsObj["globalPosition"]; exists {
			switch v := gpVal.(type) {
			case float64:
				gp := int64(v)
				opts.GlobalPosition = &gp
			case int:
				gp := int64(v)
				opts.GlobalPosition = &gp
			case int64:
				opts.GlobalPosition = &v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.globalPosition must be a number",
				}
			}
		}

		// Parse batchSize
		if bsVal, exists := optsObj["batchSize"]; exists {
			switch v := bsVal.(type) {
			case float64:
				opts.BatchSize = int64(v)
			case int:
				opts.BatchSize = int64(v)
			case int64:
				opts.BatchSize = v
			default:
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.batchSize must be a number",
				}
			}

			// Validate batch size
			if opts.BatchSize > 10000 && opts.BatchSize != -1 {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.batchSize must be <= 10000 or -1 for unlimited",
				}
			}
		}

		// Parse correlation filter
		if corrVal, exists := optsObj["correlation"]; exists {
			corrStr, ok := corrVal.(string)
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.correlation must be a string",
				}
			}
			opts.Correlation = &corrStr
		}

		// Parse consumer group
		if cgVal, exists := optsObj["consumerGroup"]; exists {
			cgObj, ok := cgVal.(map[string]interface{})
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.consumerGroup must be an object",
				}
			}

			// Parse member
			if memberVal, exists := cgObj["member"]; exists {
				switch v := memberVal.(type) {
				case float64:
					member := int64(v)
					opts.ConsumerMember = &member
				case int:
					member := int64(v)
					opts.ConsumerMember = &member
				case int64:
					opts.ConsumerMember = &v
				default:
					return nil, &RPCError{
						Code:    "INVALID_REQUEST",
						Message: "options.consumerGroup.member must be a number",
					}
				}
			}

			// Parse size
			if sizeVal, exists := cgObj["size"]; exists {
				switch v := sizeVal.(type) {
				case float64:
					size := int64(v)
					opts.ConsumerSize = &size
				case int:
					size := int64(v)
					opts.ConsumerSize = &size
				case int64:
					opts.ConsumerSize = &v
				default:
					return nil, &RPCError{
						Code:    "INVALID_REQUEST",
						Message: "options.consumerGroup.size must be a number",
					}
				}
			}

			// Validate consumer group parameters
			if opts.ConsumerMember != nil && opts.ConsumerSize != nil {
				if *opts.ConsumerMember < 0 {
					return nil, &RPCError{
						Code:    "INVALID_REQUEST",
						Message: "options.consumerGroup.member must be non-negative",
					}
				}
				if *opts.ConsumerSize <= 0 {
					return nil, &RPCError{
						Code:    "INVALID_REQUEST",
						Message: "options.consumerGroup.size must be positive",
					}
				}
				if *opts.ConsumerMember >= *opts.ConsumerSize {
					return nil, &RPCError{
						Code:    "INVALID_REQUEST",
						Message: "options.consumerGroup.member must be < size",
					}
				}
			} else if opts.ConsumerMember != nil || opts.ConsumerSize != nil {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.consumerGroup requires both member and size",
				}
			}
		}
	}

	// Get namespace from context
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	// Get category messages
	messages, err := h.store.GetCategoryMessages(ctx, namespace, categoryName, opts)
	if err != nil {
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to get category messages: %v", err),
		}
	}

	// Format response as array of arrays
	// Note: For category queries, we include the stream name in the response
	result := make([]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = []interface{}{
			msg.ID,
			msg.StreamName, // Include stream name for category queries
			msg.Type,
			msg.Position,
			msg.GlobalPosition,
			msg.Data,
			msg.Metadata,
			msg.Time.UTC().Format(time.RFC3339Nano),
		}
	}

	return result, nil
}

// handleNamespaceCreate creates a new namespace
// Request: ["ns.create", "namespace-id", {opts}]
// opts.token: optional token to use (must be valid format for namespace)
// Response: {"namespace": "tenant-a", "token": "ns_...", "createdAt": "..."}
func (h *RPCHandler) handleNamespaceCreate(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "ns.create requires at least 1 argument: namespace ID",
		}
	}

	// Parse namespace ID
	namespaceID, ok := args[0].(string)
	if !ok || namespaceID == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "namespace ID must be a non-empty string",
		}
	}

	// Parse optional options
	description := ""
	var providedToken string

	if len(args) > 1 {
		optsObj, ok := args[1].(map[string]interface{})
		if !ok {
			return nil, &RPCError{
				Code:    "INVALID_REQUEST",
				Message: "options must be an object",
			}
		}

		// Extract description
		if descVal, exists := optsObj["description"]; exists {
			description, ok = descVal.(string)
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.description must be a string",
				}
			}
		}

		// Extract token (optional - allows caller to specify their own token)
		if tokenVal, exists := optsObj["token"]; exists {
			providedToken, ok = tokenVal.(string)
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.token must be a string",
				}
			}
			// Validate token format and namespace match
			tokenNS, err := auth.ParseToken(providedToken)
			if err != nil {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: fmt.Sprintf("Invalid token format: %v", err),
				}
			}
			if tokenNS != namespaceID {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: fmt.Sprintf("Token is for namespace '%s', not '%s'", tokenNS, namespaceID),
				}
			}
		}

		// Extract metadata (for future use)
		if metaVal, exists := optsObj["metadata"]; exists {
			_, ok = metaVal.(map[string]interface{})
			if !ok {
				return nil, &RPCError{
					Code:    "INVALID_REQUEST",
					Message: "options.metadata must be an object",
				}
			}
		}
	}

	// Use provided token or generate one
	var token string
	var err error
	if providedToken != "" {
		token = providedToken
	} else {
		token, err = auth.GenerateToken(namespaceID)
		if err != nil {
			return nil, &RPCError{
				Code:    "BACKEND_ERROR",
				Message: fmt.Sprintf("Failed to generate token: %v", err),
			}
		}
	}

	tokenHash := auth.HashToken(token)

	// Create namespace
	if err := h.store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		// Check for specific error types
		if errors.Is(err, store.ErrNamespaceExists) {
			return nil, &RPCError{
				Code:    "NAMESPACE_EXISTS",
				Message: fmt.Sprintf("Namespace '%s' already exists", namespaceID),
			}
		}

		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to create namespace: %v", err),
		}
	}

	// Return result
	return map[string]interface{}{
		"namespace": namespaceID,
		"token":     token,
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// handleNamespaceDelete deletes a namespace and all its data
// Request: ["ns.delete", "namespace-id"]
// Response: {"namespace": "tenant-a", "deletedAt": "...", "messagesDeleted": 1543}
func (h *RPCHandler) handleNamespaceDelete(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "ns.delete requires 1 argument: namespace ID",
		}
	}

	// Parse namespace ID
	namespaceID, ok := args[0].(string)
	if !ok || namespaceID == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "namespace ID must be a non-empty string",
		}
	}

	// Get namespace from context (auth middleware should set this)
	// For now, we use "default" - TODO: implement proper context extraction
	// contextNamespace := "default"

	// Verify token matches namespace - this should be done by auth middleware
	// For now, we allow deletion if the request is made

	// Get namespace info before deletion (for message count)
	// This is optional - we'll return 0 for now as we don't have an easy way to count
	messagesDeleted := int64(0)

	// Delete namespace
	if err := h.store.DeleteNamespace(ctx, namespaceID); err != nil {
		// Check for specific error types
		if errors.Is(err, store.ErrNamespaceNotFound) {
			return nil, &RPCError{
				Code:    "NAMESPACE_NOT_FOUND",
				Message: fmt.Sprintf("Namespace '%s' not found", namespaceID),
			}
		}

		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to delete namespace: %v", err),
		}
	}

	// Return result
	return map[string]interface{}{
		"namespace":       namespaceID,
		"deletedAt":       time.Now().UTC().Format(time.RFC3339Nano),
		"messagesDeleted": messagesDeleted,
	}, nil
}

// handleNamespaceList lists all namespaces
// Request: ["ns.list", {opts}]
// Response: [{"namespace": "default", "description": "...", "createdAt": "...", "messageCount": 1234}, ...]
func (h *RPCHandler) handleNamespaceList(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Parse optional options (limit, offset - not implemented yet)
	// For now, we ignore options and return all namespaces

	// Get all namespaces
	namespaces, err := h.store.ListNamespaces(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to list namespaces: %v", err),
		}
	}

	// Format response
	result := make([]interface{}, len(namespaces))
	for i, ns := range namespaces {
		result[i] = map[string]interface{}{
			"namespace":    ns.ID,
			"description":  ns.Description,
			"createdAt":    ns.CreatedAt.UTC().Format(time.RFC3339Nano),
			"messageCount": 0, // TODO: implement message counting
		}
	}

	return result, nil
}

// handleNamespaceInfo returns information about a namespace
// Request: ["ns.info", "namespace-id"]
// Response: {"namespace": "tenant-a", "description": "...", "createdAt": "...", "messageCount": 567, "streamCount": 12, "lastActivity": "..."}
func (h *RPCHandler) handleNamespaceInfo(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	// Validate arguments
	if len(args) < 1 {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "ns.info requires 1 argument: namespace ID",
		}
	}

	// Parse namespace ID
	namespaceID, ok := args[0].(string)
	if !ok || namespaceID == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "namespace ID must be a non-empty string",
		}
	}

	// Get namespace
	ns, err := h.store.GetNamespace(ctx, namespaceID)
	if err != nil {
		// Check for specific error types
		if errors.Is(err, store.ErrNamespaceNotFound) {
			return nil, &RPCError{
				Code:    "NAMESPACE_NOT_FOUND",
				Message: fmt.Sprintf("Namespace '%s' not found", namespaceID),
			}
		}

		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to get namespace: %v", err),
		}
	}

	// Get message count
	messageCount, err := h.store.GetNamespaceMessageCount(ctx, namespaceID)
	if err != nil {
		// If we can't get the count, just return 0 rather than failing
		messageCount = 0
	}

	// Return result
	// TODO: Implement streamCount and lastActivity
	return map[string]interface{}{
		"namespace":    ns.ID,
		"description":  ns.Description,
		"createdAt":    ns.CreatedAt.UTC().Format(time.RFC3339Nano),
		"messageCount": messageCount,
		"streamCount":  0,
		"lastActivity": nil,
	}, nil
}

// handleNamespaceStreams lists streams in the current namespace
// Request: ["ns.streams", {opts}]
// Response: [{"stream": "...", "version": 5, "lastActivity": "..."}, ...]
func (h *RPCHandler) handleNamespaceStreams(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	opts := &store.ListStreamsOpts{Limit: 100}

	if len(args) > 0 {
		optsObj, ok := args[0].(map[string]interface{})
		if !ok {
			return nil, &RPCError{Code: "INVALID_REQUEST", Message: "options must be an object"}
		}

		if v, exists := optsObj["prefix"]; exists {
			s, ok := v.(string)
			if !ok {
				return nil, &RPCError{Code: "INVALID_REQUEST", Message: "prefix must be a string"}
			}
			opts.Prefix = s
		}
		if v, exists := optsObj["cursor"]; exists {
			s, ok := v.(string)
			if !ok {
				return nil, &RPCError{Code: "INVALID_REQUEST", Message: "cursor must be a string"}
			}
			opts.Cursor = s
		}
		if v, exists := optsObj["limit"]; exists {
			switch n := v.(type) {
			case float64:
				opts.Limit = int64(n)
			case int:
				opts.Limit = int64(n)
			case int64:
				opts.Limit = n
			default:
				return nil, &RPCError{Code: "INVALID_REQUEST", Message: "limit must be a number"}
			}
			if opts.Limit <= 0 || opts.Limit > 1000 {
				return nil, &RPCError{Code: "INVALID_REQUEST", Message: "limit must be between 1 and 1000"}
			}
		}
	}

	streams, err := h.store.ListStreams(ctx, namespace, opts)
	if err != nil {
		return nil, &RPCError{Code: "BACKEND_ERROR", Message: fmt.Sprintf("Failed to list streams: %v", err)}
	}

	result := make([]interface{}, len(streams))
	for i, s := range streams {
		result[i] = map[string]interface{}{
			"stream":       s.StreamName,
			"version":      s.Version,
			"lastActivity": s.LastActivity.UTC().Format(time.RFC3339),
		}
	}
	return result, nil
}

// handleNamespaceCategories lists distinct categories in the current namespace
// Request: ["ns.categories"]
// Response: [{"category": "...", "streamCount": 42, "messageCount": 1500}, ...]
func (h *RPCHandler) handleNamespaceCategories(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	namespace, rpcErr := h.getNamespace(ctx)
	if rpcErr != nil {
		return nil, rpcErr
	}

	categories, err := h.store.ListCategories(ctx, namespace)
	if err != nil {
		return nil, &RPCError{Code: "BACKEND_ERROR", Message: fmt.Sprintf("Failed to list categories: %v", err)}
	}

	result := make([]interface{}, len(categories))
	for i, c := range categories {
		result[i] = map[string]interface{}{
			"category":     c.Category,
			"streamCount":  c.StreamCount,
			"messageCount": c.MessageCount,
		}
	}
	return result, nil
}

// getNamespace extracts namespace from context, with auto-creation support in test mode
func (h *RPCHandler) getNamespace(ctx context.Context) (string, *RPCError) {
	// Try to get namespace from context (set by auth middleware)
	namespace, ok := GetNamespaceFromContext(ctx)
	if ok {
		return namespace, nil
	}

	// In test mode, use default namespace
	if IsTestMode(ctx) {
		namespace = "default"
		// Auto-create namespace if it doesn't exist
		if err := h.ensureNamespaceInTestMode(ctx, namespace); err != nil {
			return "", &RPCError{
				Code:    "BACKEND_ERROR",
				Message: fmt.Sprintf("Failed to create default namespace: %v", err),
			}
		}
		return namespace, nil
	}

	// No namespace found and not in test mode
	return "", &RPCError{
		Code:    "AUTH_REQUIRED",
		Message: "No namespace found in context",
	}
}

// ensureNamespaceInTestMode creates a namespace if it doesn't exist (test mode only)
// This function is thread-safe and handles concurrent creation attempts gracefully
func (h *RPCHandler) ensureNamespaceInTestMode(ctx context.Context, namespace string) error {
	// First check if namespace exists (fast path)
	_, err := h.store.GetNamespace(ctx, namespace)
	if err == nil {
		// Namespace already exists
		return nil
	}

	// Lock to serialize namespace creation attempts
	h.nsMu.Lock()
	defer h.nsMu.Unlock()

	// Double-check if namespace exists (another goroutine might have created it)
	_, err = h.store.GetNamespace(ctx, namespace)
	if err == nil {
		// Namespace was created by another goroutine while we waited for the lock
		return nil
	}

	// Create namespace with a test token hash
	tokenHash := "test-mode-hash-" + namespace
	err = h.store.CreateNamespace(ctx, namespace, tokenHash, "Auto-created in test mode")

	// If the namespace already exists (race at store level), that's fine
	if err != nil && errors.Is(err, store.ErrNamespaceExists) {
		return nil
	}

	return err
}
