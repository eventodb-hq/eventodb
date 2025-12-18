// Package api provides HTTP handlers for the RPC API.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/message-db/message-db/internal/logger"
	"github.com/message-db/message-db/internal/store"
)

// RPCHandler handles RPC requests in array format: ["method", arg1, arg2, ...]
type RPCHandler struct {
	version string
	store   store.Store
	pubsub  *PubSub
	methods map[string]RPCMethod
	nsMu    sync.Mutex // Protects namespace auto-creation in test mode
}

// RPCMethod is a function that handles an RPC method call
type RPCMethod func(ctx context.Context, args []interface{}) (interface{}, *RPCError)

// RPCError represents an RPC error response
type RPCError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ErrorResponse wraps an RPCError for JSON serialization
type ErrorResponse struct {
	Error *RPCError `json:"error"`
}

// NewRPCHandler creates a new RPC handler
func NewRPCHandler(version string, st store.Store, pubsub *PubSub) *RPCHandler {
	h := &RPCHandler{
		version: version,
		store:   st,
		pubsub:  pubsub,
		methods: make(map[string]RPCMethod),
	}

	// Register system methods
	h.registerMethod("sys.version", h.handleSysVersion)
	h.registerMethod("sys.health", h.handleSysHealth)

	// Register stream methods
	h.registerMethod("stream.write", h.handleStreamWrite)
	h.registerMethod("stream.get", h.handleStreamGet)
	h.registerMethod("stream.last", h.handleStreamLast)
	h.registerMethod("stream.version", h.handleStreamVersion)

	// Register category methods
	h.registerMethod("category.get", h.handleCategoryGet)

	// Register namespace methods
	h.registerMethod("ns.create", h.handleNamespaceCreate)
	h.registerMethod("ns.delete", h.handleNamespaceDelete)
	h.registerMethod("ns.list", h.handleNamespaceList)
	h.registerMethod("ns.info", h.handleNamespaceInfo)

	return h
}

// registerMethod registers an RPC method handler
func (h *RPCHandler) registerMethod(name string, handler RPCMethod) {
	h.methods[name] = handler
}

// ServeHTTP implements http.Handler
func (h *RPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Get().Debug().Msg("RPC ServeHTTP called")
	// Only accept POST requests
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Only POST method allowed",
		})
		return
	}

	// Parse request body as JSON array
	var req []interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Get().Error().Err(err).Msg("JSON parse error")
		h.writeError(w, http.StatusBadRequest, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Malformed JSON request",
			Details: map[string]interface{}{"error": err.Error()},
		})
		return
	}

	// Validate request format
	if len(req) < 1 {
		h.writeError(w, http.StatusBadRequest, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Missing method name",
		})
		return
	}

	// Extract method name
	method, ok := req[0].(string)
	if !ok {
		h.writeError(w, http.StatusBadRequest, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Method name must be a string",
		})
		return
	}

	// Extract arguments
	args := []interface{}{}
	if len(req) > 1 {
		args = req[1:]
	}

	// Route to handler
	logger.Get().Debug().
		Str("method", method).
		Int("args_count", len(args)).
		Msg("RPC method invoked")
	result, err := h.route(r.Context(), method, args)
	if err != nil {
		// Determine HTTP status code based on error code
		statusCode := http.StatusInternalServerError
		switch err.Code {
		case "INVALID_REQUEST":
			statusCode = http.StatusBadRequest
		case "METHOD_NOT_FOUND":
			statusCode = http.StatusNotFound
		case "AUTH_REQUIRED", "AUTH_INVALID_TOKEN":
			statusCode = http.StatusUnauthorized
		case "AUTH_UNAUTHORIZED":
			statusCode = http.StatusForbidden
		case "STREAM_NOT_FOUND", "NAMESPACE_NOT_FOUND":
			statusCode = http.StatusNotFound
		case "STREAM_VERSION_CONFLICT", "NAMESPACE_EXISTS":
			statusCode = http.StatusConflict
		}
		if statusCode == http.StatusInternalServerError {
			logger.Get().Error().
				Str("method", method).
				Str("error_code", err.Code).
				Str("error_message", err.Message).
				Msg("RPC internal server error")
		}
		h.writeError(w, statusCode, err)
		return
	}

	// Write success response
	h.writeSuccess(w, result)
}

// route dispatches the request to the appropriate method handler
func (h *RPCHandler) route(ctx context.Context, method string, args []interface{}) (interface{}, *RPCError) {
	handler, exists := h.methods[method]
	if !exists {
		return nil, &RPCError{
			Code:    "METHOD_NOT_FOUND",
			Message: fmt.Sprintf("Unknown method: %s", method),
		}
	}

	return handler(ctx, args)
}

// handleSysVersion returns the server version
func (h *RPCHandler) handleSysVersion(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	return h.version, nil
}

// handleSysHealth returns server health status
func (h *RPCHandler) handleSysHealth(ctx context.Context, args []interface{}) (interface{}, *RPCError) {
	backend := "none"
	if h.store != nil {
		// Determine backend type - this is a simple heuristic
		// In a real implementation, the store would expose its type
		backend = "unknown"
	}

	return map[string]interface{}{
		"status":      "ok",
		"backend":     backend,
		"connections": 0,
	}, nil
}

// writeSuccess writes a successful JSON response
func (h *RPCHandler) writeSuccess(w http.ResponseWriter, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		logger.Get().Error().Err(err).Msg("Error encoding response")
	}
}

// writeError writes an error JSON response
func (h *RPCHandler) writeError(w http.ResponseWriter, statusCode int, rpcErr *RPCError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{Error: rpcErr}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Get().Error().Err(err).Msg("Error encoding error response")
	}
}
