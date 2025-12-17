// Package api provides HTTP middleware for the RPC API.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store"
)

// LoggingMiddleware logs HTTP requests with timing information
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request details
		duration := time.Since(start)
		log.Printf("%s %s %d %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// ContextKeyNamespace is the context key for the namespace ID
	ContextKeyNamespace contextKey = "namespace"
	// ContextKeyTestMode is the context key for test mode flag
	ContextKeyTestMode contextKey = "testMode"
)

// NamespaceGetter is an interface for retrieving namespace information
type NamespaceGetter interface {
	GetNamespace(ctx context.Context, id string) (*store.Namespace, error)
}

// AuthMiddleware validates authentication tokens and adds namespace to context
func AuthMiddleware(st NamespaceGetter, testMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// In test mode, auth is optional
			if testMode {
				ctx := context.WithValue(r.Context(), ContextKeyTestMode, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, &RPCError{
					Code:    "AUTH_REQUIRED",
					Message: "Authorization header required",
				})
				return
			}

			// Validate Bearer token format
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeAuthError(w, http.StatusUnauthorized, &RPCError{
					Code:    "AUTH_REQUIRED",
					Message: "Authorization header must use Bearer scheme",
				})
				return
			}

			// Extract token
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Parse token to extract namespace
			namespace, err := auth.ParseToken(token)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, &RPCError{
					Code:    "AUTH_INVALID_TOKEN",
					Message: "Invalid token format",
					Details: map[string]interface{}{"error": err.Error()},
				})
				return
			}

			// Validate token against database
			tokenHash := auth.HashToken(token)
			ns, err := st.GetNamespace(r.Context(), namespace)
			if err != nil {
				writeAuthError(w, http.StatusForbidden, &RPCError{
					Code:    "AUTH_UNAUTHORIZED",
					Message: "Token not authorized for namespace",
					Details: map[string]interface{}{"namespace": namespace},
				})
				return
			}

			// Verify token hash matches
			if ns.TokenHash != tokenHash {
				writeAuthError(w, http.StatusForbidden, &RPCError{
					Code:    "AUTH_UNAUTHORIZED",
					Message: "Token not authorized for namespace",
					Details: map[string]interface{}{"namespace": namespace},
				})
				return
			}

			// Add namespace to context
			ctx := context.WithValue(r.Context(), ContextKeyNamespace, namespace)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeAuthError writes an authentication error response
func writeAuthError(w http.ResponseWriter, statusCode int, rpcErr *RPCError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{Error: rpcErr}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
}

// GetNamespaceFromContext retrieves the namespace from the request context
func GetNamespaceFromContext(ctx context.Context) (string, bool) {
	namespace, ok := ctx.Value(ContextKeyNamespace).(string)
	return namespace, ok
}

// IsTestMode checks if the request is in test mode
func IsTestMode(ctx context.Context) bool {
	testMode, ok := ctx.Value(ContextKeyTestMode).(bool)
	return ok && testMode
}
