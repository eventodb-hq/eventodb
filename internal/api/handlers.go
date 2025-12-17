// Package api provides RPC method handlers for stream operations.
package api

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store"
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
		msgID = uuid.New().String()
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
		if err.Error() == "stream not found" {
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

	// Parse category name
	categoryName, ok := args[0].(string)
	if !ok || categoryName == "" {
		return nil, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "categoryName must be a non-empty string",
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
	// Note: metadata is parsed but not currently used in namespace creation
	// It's available for future use

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

	// Generate token and hash
	token, err := auth.GenerateToken(namespaceID)
	if err != nil {
		return nil, &RPCError{
			Code:    "BACKEND_ERROR",
			Message: fmt.Sprintf("Failed to generate token: %v", err),
		}
	}

	tokenHash := auth.HashToken(token)

	// Create namespace
	if err := h.store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		// Check for specific error types
		if err.Error() == "namespace already exists" {
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
		if err.Error() == "namespace not found" {
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
		if err.Error() == "namespace not found" {
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

	// Return result
	// TODO: Implement actual counts and lastActivity
	return map[string]interface{}{
		"namespace":    ns.ID,
		"description":  ns.Description,
		"createdAt":    ns.CreatedAt.UTC().Format(time.RFC3339Nano),
		"messageCount": 0,
		"streamCount":  0,
		"lastActivity": nil,
	}, nil
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
func (h *RPCHandler) ensureNamespaceInTestMode(ctx context.Context, namespace string) error {
	// Check if namespace exists
	_, err := h.store.GetNamespace(ctx, namespace)
	if err == nil {
		// Namespace exists
		return nil
	}

	// Create namespace with a test token hash
	tokenHash := "test-mode-hash-" + namespace
	return h.store.CreateNamespace(ctx, namespace, tokenHash, "Auto-created in test mode")
}
