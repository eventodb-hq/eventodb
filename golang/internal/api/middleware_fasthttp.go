// Package api provides fasthttp middleware for the RPC API.
package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/logger"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

// LoggingMiddlewareFast logs HTTP requests with timing information (fasthttp version)
func LoggingMiddlewareFast(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		start := time.Now()

		// Call next handler
		next(ctx)

		// Log request details
		duration := time.Since(start)
		log := logger.Get()

		statusCode := ctx.Response.StatusCode()
		event := log.WithLevel(zerolog.InfoLevel)
		if statusCode >= 500 {
			event = log.Error()
		}

		event.
			Str("method", string(ctx.Method())).
			Str("path", string(ctx.Path())).
			Int("status", statusCode).
			Dur("duration", duration).
			Msg("HTTP request")
	}
}

// AuthMiddlewareFast validates authentication tokens and adds namespace to context (fasthttp version)
func AuthMiddlewareFast(st NamespaceGetter, testMode bool) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			reqCtx := context.Background()
			if testMode {
				reqCtx = context.WithValue(reqCtx, ContextKeyTestMode, true)
			}

			// Extract token - first check query parameter (for SSE), then Authorization header
			var token string
			
			// Check query parameter first (for SSE subscriptions which can't set custom headers)
			if tokenParam := string(ctx.QueryArgs().Peek("token")); tokenParam != "" {
				token = tokenParam
			} else {
				// Extract Authorization header
				authHeader := string(ctx.Request.Header.Peek("Authorization"))
				if authHeader == "" {
					// In test mode, missing auth is allowed
					if testMode {
						ctx.SetUserValue("namespace", "default")
						ctx.SetUserValue("testMode", true)
						next(ctx)
						return
					}
					writeAuthErrorFast(ctx, fasthttp.StatusUnauthorized, &RPCError{
						Code:    "AUTH_REQUIRED",
						Message: "Authorization header required",
					})
					return
				}

				// Validate Bearer token format
				if !strings.HasPrefix(authHeader, "Bearer ") {
					if testMode {
						// In test mode, ignore invalid format
						ctx.SetUserValue("namespace", "default")
						ctx.SetUserValue("testMode", true)
						next(ctx)
						return
					}
					writeAuthErrorFast(ctx, fasthttp.StatusUnauthorized, &RPCError{
						Code:    "AUTH_REQUIRED",
						Message: "Authorization header must use Bearer scheme",
					})
					return
				}

				// Extract token from Bearer scheme
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// If we still don't have a token, fail
			if token == "" {
				if testMode {
					ctx.SetUserValue("namespace", "default")
					ctx.SetUserValue("testMode", true)
					next(ctx)
					return
				}
				writeAuthErrorFast(ctx, fasthttp.StatusUnauthorized, &RPCError{
					Code:    "AUTH_REQUIRED",
					Message: "Token required",
				})
				return
			}

			// Parse token to extract namespace
			namespace, err := auth.ParseToken(token)
			if err != nil {
				if testMode {
					// In test mode, ignore parse errors
					ctx.SetUserValue("namespace", "default")
					ctx.SetUserValue("testMode", true)
					next(ctx)
					return
				}
				writeAuthErrorFast(ctx, fasthttp.StatusUnauthorized, &RPCError{
					Code:    "AUTH_INVALID_TOKEN",
					Message: "Invalid token format",
					Details: map[string]interface{}{"error": err.Error()},
				})
				return
			}

			// Validate token against database (skip in test mode if namespace doesn't exist)
			tokenHash := auth.HashToken(token)
			ns, err := st.GetNamespace(reqCtx, namespace)
			if err != nil {
				if testMode {
					// In test mode, allow non-existent namespaces - they'll be auto-created
					ctx.SetUserValue("namespace", namespace)
					ctx.SetUserValue("testMode", true)
					next(ctx)
					return
				}
				writeAuthErrorFast(ctx, fasthttp.StatusForbidden, &RPCError{
					Code:    "AUTH_UNAUTHORIZED",
					Message: "Token not authorized for namespace",
					Details: map[string]interface{}{"namespace": namespace},
				})
				return
			}

			// Verify token hash matches (skip in test mode)
			if !testMode && ns.TokenHash != tokenHash {
				writeAuthErrorFast(ctx, fasthttp.StatusForbidden, &RPCError{
					Code:    "AUTH_UNAUTHORIZED",
					Message: "Token not authorized for namespace",
					Details: map[string]interface{}{"namespace": namespace},
				})
				return
			}

			// Add namespace to user values
			ctx.SetUserValue("namespace", namespace)
			next(ctx)
		}
	}
}

// writeAuthErrorFast writes an authentication error response (fasthttp version)
func writeAuthErrorFast(ctx *fasthttp.RequestCtx, statusCode int, rpcErr *RPCError) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)

	resp := ErrorResponse{Error: rpcErr}
	if err := json.NewEncoder(ctx).Encode(resp); err != nil {
		logger.Get().Error().Err(err).Msg("Error encoding error response")
	}
}

// GetNamespaceFromFastHTTP retrieves the namespace from fasthttp user values
func GetNamespaceFromFastHTTP(ctx *fasthttp.RequestCtx) (string, bool) {
	if v := ctx.UserValue("namespace"); v != nil {
		if ns, ok := v.(string); ok {
			return ns, true
		}
	}
	return "", false
}

// IsTestModeFastHTTP checks if the request is in test mode (fasthttp version)
func IsTestModeFastHTTP(ctx *fasthttp.RequestCtx) bool {
	if v := ctx.UserValue("testMode"); v != nil {
		if tm, ok := v.(bool); ok {
			return tm
		}
	}
	return false
}
