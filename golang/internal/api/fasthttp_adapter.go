// Package api provides adapters for fasthttp compatibility
package api

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"

	"github.com/valyala/fasthttp"
)

// FastHTTPHandler wraps an http.Handler to work with fasthttp
func FastHTTPHandler(h http.Handler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Convert fasthttp request to http.Request
		req := convertRequest(ctx)

		// Create response writer wrapper
		w := &fasthttpResponseWriter{
			ctx:    ctx,
			header: make(http.Header),
		}

		// Call the handler
		h.ServeHTTP(w, req)
	}
}

// convertRequest converts a fasthttp request to net/http request
func convertRequest(ctx *fasthttp.RequestCtx) *http.Request {
	// Create a reader from the request body
	bodyReader := bytes.NewReader(ctx.Request.Body())

	// Build URL
	uri := ctx.Request.URI()
	scheme := "http"
	if ctx.IsTLS() {
		scheme = "https"
	}
	url := scheme + "://" + string(uri.Host()) + string(uri.Path())
	if len(uri.QueryString()) > 0 {
		url += "?" + string(uri.QueryString())
	}

	// Create http.Request
	req, _ := http.NewRequestWithContext(
		context.Background(),
		string(ctx.Method()),
		url,
		bodyReader,
	)

	// Copy headers
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		req.Header.Add(string(key), string(value))
	})

	return req
}

// AdaptRequestToStdlib converts a fasthttp.RequestCtx to standard library compatible objects
// This is used for SSE where we need ResponseWriter interface features
// It also transfers namespace and test mode context from fasthttp user values
func AdaptRequestToStdlib(ctx *fasthttp.RequestCtx) (*http.Request, http.ResponseWriter) {
	// Create a reader from the request body
	bodyReader := bytes.NewReader(ctx.Request.Body())

	// Build URL
	uri := ctx.Request.URI()
	scheme := "http"
	if ctx.IsTLS() {
		scheme = "https"
	}
	url := scheme + "://" + string(uri.Host()) + string(uri.Path())
	if len(uri.QueryString()) > 0 {
		url += "?" + string(uri.QueryString())
	}

	// Create request context with namespace and test mode from fasthttp user values
	reqCtx := context.Background()

	// Transfer namespace from fasthttp user values to context
	if namespace, ok := GetNamespaceFromFastHTTP(ctx); ok {
		reqCtx = context.WithValue(reqCtx, ContextKeyNamespace, namespace)
	}

	// Transfer test mode from fasthttp user values to context
	if IsTestModeFastHTTP(ctx) {
		reqCtx = context.WithValue(reqCtx, ContextKeyTestMode, true)
	}

	// Create http.Request with context
	req, _ := http.NewRequestWithContext(
		reqCtx,
		string(ctx.Method()),
		url,
		bodyReader,
	)

	// Copy headers
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		req.Header.Add(string(key), string(value))
	})

	// Create a custom ResponseWriter that writes to fasthttp.RequestCtx
	w := &fasthttpResponseWriter{
		ctx:    ctx,
		header: make(http.Header),
	}

	return req, w
}

// fasthttpResponseWriter implements http.ResponseWriter for fasthttp compatibility
type fasthttpResponseWriter struct {
	ctx           *fasthttp.RequestCtx
	header        http.Header
	statusCode    int
	headerWritten bool
}

func (w *fasthttpResponseWriter) Header() http.Header {
	return w.header
}

func (w *fasthttpResponseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true
	w.statusCode = statusCode
	w.ctx.SetStatusCode(statusCode)

	// Copy headers to fasthttp response
	for key, values := range w.header {
		for _, value := range values {
			w.ctx.Response.Header.Add(key, value)
		}
	}
}

func (w *fasthttpResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.ctx.Write(b)
}

// Flush implements http.Flusher for SSE support
func (w *fasthttpResponseWriter) Flush() {
	// fasthttp doesn't buffer, so this is a no-op
	// But we implement it for compatibility
}

// hijackConn implements http.Hijacker (optional)
type hijackConn struct {
	io.ReadWriteCloser
}

// Hijack implements http.Hijacker
func (w *fasthttpResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Not fully supported but we can return a dummy implementation
	// for SSE we don't actually need hijacking
	return nil, nil, http.ErrNotSupported
}
