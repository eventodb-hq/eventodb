// Package api provides fasthttp wrapper for RPC handler
package api

import (
	"context"

	"github.com/valyala/fasthttp"
)

// FastHTTPRPCHandler wraps the RPC handler with context management for fasthttp
func FastHTTPRPCHandler(h *RPCHandler, testMode bool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Build context with namespace and test mode
		reqCtx := context.Background()

		if namespace, ok := GetNamespaceFromFastHTTP(ctx); ok {
			reqCtx = context.WithValue(reqCtx, ContextKeyNamespace, namespace)
		}

		if IsTestModeFastHTTP(ctx) {
			reqCtx = context.WithValue(reqCtx, ContextKeyTestMode, true)
		}

		// Store context in fasthttp user values for handlers to access
		ctx.SetUserValue("ctx", reqCtx)

		// Call the fast handler
		h.ServeHTTPFast(ctx)
	}
}


