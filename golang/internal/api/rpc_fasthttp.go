// Package api provides fasthttp-native handlers for the RPC API.
package api

import (
	"context"
	"encoding/json"

	"github.com/eventodb/eventodb/internal/logger"
	"github.com/valyala/fasthttp"
)

// ServeHTTPFast handles RPC requests using fasthttp natively
func (h *RPCHandler) ServeHTTPFast(ctx *fasthttp.RequestCtx) {
	logger.Get().Debug().Msg("RPC ServeHTTPFast called")

	// Only accept POST requests
	if !ctx.IsPost() {
		h.writeErrorFast(ctx, fasthttp.StatusMethodNotAllowed, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Only POST method allowed",
		})
		return
	}

	// Parse request body as JSON array
	var req []interface{}
	if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
		logger.Get().Error().Err(err).Msg("JSON parse error")
		h.writeErrorFast(ctx, fasthttp.StatusBadRequest, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Malformed JSON request",
			Details: map[string]interface{}{"error": err.Error()},
		})
		return
	}

	// Validate request format
	if len(req) < 1 {
		h.writeErrorFast(ctx, fasthttp.StatusBadRequest, &RPCError{
			Code:    "INVALID_REQUEST",
			Message: "Missing method name",
		})
		return
	}

	// Extract method name
	method, ok := req[0].(string)
	if !ok {
		h.writeErrorFast(ctx, fasthttp.StatusBadRequest, &RPCError{
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

	// Get context from user values
	reqCtx := context.Background()
	if ctxVal := ctx.UserValue("ctx"); ctxVal != nil {
		if c, ok := ctxVal.(context.Context); ok {
			reqCtx = c
		}
	}

	// Route to handler
	logger.Get().Debug().
		Str("method", method).
		Int("args_count", len(args)).
		Msg("RPC method invoked")

	result, err := h.route(reqCtx, method, args)
	if err != nil {
		// Determine HTTP status code based on error code
		statusCode := fasthttp.StatusInternalServerError
		switch err.Code {
		case "INVALID_REQUEST":
			statusCode = fasthttp.StatusBadRequest
		case "METHOD_NOT_FOUND":
			statusCode = fasthttp.StatusNotFound
		case "AUTH_REQUIRED", "AUTH_INVALID_TOKEN":
			statusCode = fasthttp.StatusUnauthorized
		case "AUTH_UNAUTHORIZED":
			statusCode = fasthttp.StatusForbidden
		case "STREAM_NOT_FOUND", "NAMESPACE_NOT_FOUND":
			statusCode = fasthttp.StatusNotFound
		case "STREAM_VERSION_CONFLICT", "NAMESPACE_EXISTS":
			statusCode = fasthttp.StatusConflict
		}
		if statusCode == fasthttp.StatusInternalServerError {
			logger.Get().Error().
				Str("method", method).
				Str("error_code", err.Code).
				Str("error_message", err.Message).
				Msg("RPC internal server error")
		}
		h.writeErrorFast(ctx, statusCode, err)
		return
	}

	// Write success response
	h.writeSuccessFast(ctx, result)
}

// writeSuccessFast writes a successful JSON response using fasthttp
func (h *RPCHandler) writeSuccessFast(ctx *fasthttp.RequestCtx, result interface{}) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)

	if err := json.NewEncoder(ctx).Encode(result); err != nil {
		logger.Get().Error().Err(err).Msg("Error encoding response")
	}
}

// writeErrorFast writes an error JSON response using fasthttp
func (h *RPCHandler) writeErrorFast(ctx *fasthttp.RequestCtx, statusCode int, rpcErr *RPCError) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)

	resp := ErrorResponse{Error: rpcErr}
	if err := json.NewEncoder(ctx).Encode(resp); err != nil {
		logger.Get().Error().Err(err).Msg("Error encoding error response")
	}
}
