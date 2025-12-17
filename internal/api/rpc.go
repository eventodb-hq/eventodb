// Package api provides HTTP handlers for the RPC API.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/message-db/message-db/internal/store"
)

// RPCHandler handles RPC requests in array format: ["method", arg1, arg2, ...]
type RPCHandler struct {
	version string
	store   store.Store
	methods map[string]RPCMethod
}

// RPCMethod is a function that handles an RPC method call
type RPCMethod func(args []interface{}) (interface{}, *RPCError)

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
func NewRPCHandler(version string, st store.Store) *RPCHandler {
	h := &RPCHandler{
		version: version,
		store:   st,
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

	return h
}

// registerMethod registers an RPC method handler
func (h *RPCHandler) registerMethod(name string, handler RPCMethod) {
	h.methods[name] = handler
}

// ServeHTTP implements http.Handler
func (h *RPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	result, err := h.route(method, args)
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
		h.writeError(w, statusCode, err)
		return
	}

	// Write success response
	h.writeSuccess(w, result)
}

// route dispatches the request to the appropriate method handler
func (h *RPCHandler) route(method string, args []interface{}) (interface{}, *RPCError) {
	handler, exists := h.methods[method]
	if !exists {
		return nil, &RPCError{
			Code:    "METHOD_NOT_FOUND",
			Message: fmt.Sprintf("Unknown method: %s", method),
		}
	}

	return handler(args)
}

// handleSysVersion returns the server version
func (h *RPCHandler) handleSysVersion(args []interface{}) (interface{}, *RPCError) {
	return h.version, nil
}

// handleSysHealth returns server health status
func (h *RPCHandler) handleSysHealth(args []interface{}) (interface{}, *RPCError) {
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
		log.Printf("Error encoding response: %v", err)
	}
}

// writeError writes an error JSON response
func (h *RPCHandler) writeError(w http.ResponseWriter, statusCode int, rpcErr *RPCError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	resp := ErrorResponse{Error: rpcErr}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
}
