// Package api provides the HTTP import handler for bulk event import.
package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/eventodb/eventodb/internal/logger"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/valyala/fasthttp"
)

const (
	// importBatchSize is the number of messages to buffer before batch insert
	importBatchSize = 1000
)

// ExportRecord represents the NDJSON format for export/import
type ExportRecord struct {
	ID       string                 `json:"id"`
	Stream   string                 `json:"stream"`
	Type     string                 `json:"type"`
	Position int64                  `json:"pos"`
	GPos     int64                  `json:"gpos"`
	Data     map[string]interface{} `json:"data"`
	Meta     map[string]interface{} `json:"meta"`
	Time     string                 `json:"time"`
}

// ImportProgress represents a progress event sent during import
type ImportProgress struct {
	Imported int64 `json:"imported"`
	GPos     int64 `json:"gpos"`
}

// ImportDone represents the final event when import completes
type ImportDone struct {
	Done     bool   `json:"done"`
	Imported int64  `json:"imported"`
	Elapsed  string `json:"elapsed"`
}

// ImportError represents an error event during import
type ImportError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Line    int64  `json:"line,omitempty"`
}

// ImportHandler handles streaming import of events
type ImportHandler struct {
	store store.Store
}

// NewImportHandler creates a new import handler
func NewImportHandler(st store.Store) *ImportHandler {
	return &ImportHandler{
		store: st,
	}
}

// HandleImport handles POST /import requests with streaming NDJSON body
func (h *ImportHandler) HandleImport(ctx *fasthttp.RequestCtx) {
	// Get namespace from middleware
	namespace, ok := GetNamespaceFromFastHTTP(ctx)
	if !ok {
		h.writeError(ctx, fasthttp.StatusUnauthorized, "AUTH_REQUIRED", "Namespace not found in context")
		return
	}

	// Set up SSE response headers
	ctx.SetContentType("text/event-stream")
	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("X-Accel-Buffering", "no")

	// Get request body
	body := ctx.PostBody()
	if len(body) == 0 {
		// Empty body is valid - just return done with 0 imported
		h.sendDone(ctx, 0, time.Duration(0))
		return
	}

	start := time.Now()
	scanner := bufio.NewScanner(bytes.NewReader(body))

	// Increase buffer size for large lines
	const maxLineSize = 1024 * 1024 // 1MB max line size
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	batch := make([]*store.Message, 0, importBatchSize)
	var imported int64
	var lineNum int64
	var lastGPos int64

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Parse NDJSON record
		var record ExportRecord
		if err := json.Unmarshal(line, &record); err != nil {
			h.sendError(ctx, "INVALID_JSON", fmt.Sprintf("malformed JSON at line %d: %v", lineNum, err), lineNum)
			return
		}

		// Convert to store.Message
		msg, err := h.recordToMessage(&record)
		if err != nil {
			h.sendError(ctx, "INVALID_RECORD", fmt.Sprintf("invalid record at line %d: %v", lineNum, err), lineNum)
			return
		}

		batch = append(batch, msg)
		lastGPos = msg.GlobalPosition

		// Batch insert when we reach batch size
		if len(batch) >= importBatchSize {
			if err := h.store.ImportBatch(ctx, namespace, batch); err != nil {
				h.handleImportError(ctx, err, lineNum)
				return
			}
			imported += int64(len(batch))
			h.sendProgress(ctx, imported, lastGPos)
			batch = batch[:0] // Reset batch, reuse slice
		}
	}

	// Check for scanner error
	if err := scanner.Err(); err != nil {
		h.sendError(ctx, "READ_ERROR", fmt.Sprintf("error reading input: %v", err), lineNum)
		return
	}

	// Flush remaining batch
	if len(batch) > 0 {
		if err := h.store.ImportBatch(ctx, namespace, batch); err != nil {
			h.handleImportError(ctx, err, lineNum)
			return
		}
		imported += int64(len(batch))
	}

	// Send completion event
	h.sendDone(ctx, imported, time.Since(start))

	logger.Get().Info().
		Str("namespace", namespace).
		Int64("imported", imported).
		Dur("elapsed", time.Since(start)).
		Msg("Import completed")
}

// recordToMessage converts an ExportRecord to a store.Message
func (h *ImportHandler) recordToMessage(record *ExportRecord) (*store.Message, error) {
	// Parse time
	t, err := time.Parse(time.RFC3339, record.Time)
	if err != nil {
		// Try RFC3339Nano as fallback
		t, err = time.Parse(time.RFC3339Nano, record.Time)
		if err != nil {
			return nil, fmt.Errorf("invalid time format: %w", err)
		}
	}

	return &store.Message{
		ID:             record.ID,
		StreamName:     record.Stream,
		Type:           record.Type,
		Position:       record.Position,
		GlobalPosition: record.GPos,
		Data:           record.Data,
		Metadata:       record.Meta,
		Time:           t.UTC(),
	}, nil
}

// handleImportError handles errors from ImportBatch
func (h *ImportHandler) handleImportError(ctx *fasthttp.RequestCtx, err error, lineNum int64) {
	if errors.Is(err, store.ErrPositionExists) {
		h.sendError(ctx, "POSITION_EXISTS", err.Error(), lineNum)
	} else {
		h.sendError(ctx, "IMPORT_FAILED", err.Error(), lineNum)
	}
}

// sendProgress sends a progress event
func (h *ImportHandler) sendProgress(ctx *fasthttp.RequestCtx, imported, gpos int64) {
	progress := ImportProgress{
		Imported: imported,
		GPos:     gpos,
	}
	data, _ := json.Marshal(progress)
	fmt.Fprintf(ctx, "data: %s\n\n", data)
}

// sendDone sends the completion event
func (h *ImportHandler) sendDone(ctx *fasthttp.RequestCtx, imported int64, elapsed time.Duration) {
	done := ImportDone{
		Done:     true,
		Imported: imported,
		Elapsed:  fmt.Sprintf("%.1fs", elapsed.Seconds()),
	}
	data, _ := json.Marshal(done)
	fmt.Fprintf(ctx, "data: %s\n\n", data)
}

// sendError sends an error event
func (h *ImportHandler) sendError(ctx *fasthttp.RequestCtx, code, message string, lineNum int64) {
	errResp := ImportError{
		Error:   code,
		Message: message,
	}
	if lineNum > 0 {
		errResp.Line = lineNum
	}
	data, _ := json.Marshal(errResp)
	fmt.Fprintf(ctx, "data: %s\n\n", data)
}

// writeError writes a JSON error response (for pre-streaming errors)
func (h *ImportHandler) writeError(ctx *fasthttp.RequestCtx, statusCode int, code, message string) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(statusCode)

	resp := ErrorResponse{Error: &RPCError{
		Code:    code,
		Message: message,
	}}
	json.NewEncoder(ctx).Encode(resp)
}

// ServeHTTP implements http.Handler for net/http compatibility (used in tests)
func (h *ImportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get namespace from context
	namespace, ok := GetNamespaceFromContext(r.Context())
	if !ok {
		// Try test mode default
		if IsTestMode(r.Context()) {
			namespace = "default"
		} else {
			h.writeHTTPError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Namespace not found in context")
			return
		}
	}

	// Set up SSE response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendHTTPError(w, "READ_ERROR", fmt.Sprintf("failed to read body: %v", err), 0)
		return
	}

	if len(body) == 0 {
		// Empty body is valid - just return done with 0 imported
		h.sendHTTPDone(w, 0, time.Duration(0))
		return
	}

	start := time.Now()
	scanner := bufio.NewScanner(bytes.NewReader(body))

	// Increase buffer size for large lines
	const maxLineSize = 1024 * 1024 // 1MB max line size
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	batch := make([]*store.Message, 0, importBatchSize)
	var imported int64
	var lineNum int64
	var lastGPos int64

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Parse NDJSON record
		var record ExportRecord
		if err := json.Unmarshal(line, &record); err != nil {
			h.sendHTTPError(w, "INVALID_JSON", fmt.Sprintf("malformed JSON at line %d: %v", lineNum, err), lineNum)
			return
		}

		// Convert to store.Message
		msg, err := h.recordToMessage(&record)
		if err != nil {
			h.sendHTTPError(w, "INVALID_RECORD", fmt.Sprintf("invalid record at line %d: %v", lineNum, err), lineNum)
			return
		}

		batch = append(batch, msg)
		lastGPos = msg.GlobalPosition

		// Batch insert when we reach batch size
		if len(batch) >= importBatchSize {
			if err := h.store.ImportBatch(r.Context(), namespace, batch); err != nil {
				h.handleHTTPImportError(w, err, lineNum)
				return
			}
			imported += int64(len(batch))
			h.sendHTTPProgress(w, imported, lastGPos)
			batch = batch[:0] // Reset batch, reuse slice
		}
	}

	// Check for scanner error
	if err := scanner.Err(); err != nil {
		h.sendHTTPError(w, "READ_ERROR", fmt.Sprintf("error reading input: %v", err), lineNum)
		return
	}

	// Flush remaining batch
	if len(batch) > 0 {
		if err := h.store.ImportBatch(r.Context(), namespace, batch); err != nil {
			h.handleHTTPImportError(w, err, lineNum)
			return
		}
		imported += int64(len(batch))
	}

	// Send completion event
	h.sendHTTPDone(w, imported, time.Since(start))

	logger.Get().Info().
		Str("namespace", namespace).
		Int64("imported", imported).
		Dur("elapsed", time.Since(start)).
		Msg("Import completed")
}

// handleHTTPImportError handles errors from ImportBatch (net/http version)
func (h *ImportHandler) handleHTTPImportError(w http.ResponseWriter, err error, lineNum int64) {
	if errors.Is(err, store.ErrPositionExists) {
		h.sendHTTPError(w, "POSITION_EXISTS", err.Error(), lineNum)
	} else {
		h.sendHTTPError(w, "IMPORT_FAILED", err.Error(), lineNum)
	}
}

// sendHTTPProgress sends a progress event (net/http version)
func (h *ImportHandler) sendHTTPProgress(w http.ResponseWriter, imported, gpos int64) {
	progress := ImportProgress{
		Imported: imported,
		GPos:     gpos,
	}
	data, _ := json.Marshal(progress)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// sendHTTPDone sends the completion event (net/http version)
func (h *ImportHandler) sendHTTPDone(w http.ResponseWriter, imported int64, elapsed time.Duration) {
	done := ImportDone{
		Done:     true,
		Imported: imported,
		Elapsed:  fmt.Sprintf("%.1fs", elapsed.Seconds()),
	}
	data, _ := json.Marshal(done)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// sendHTTPError sends an error event (net/http version)
func (h *ImportHandler) sendHTTPError(w http.ResponseWriter, code, message string, lineNum int64) {
	errResp := ImportError{
		Error:   code,
		Message: message,
	}
	if lineNum > 0 {
		errResp.Line = lineNum
	}
	data, _ := json.Marshal(errResp)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// writeHTTPError writes a JSON error response (net/http version)
func (h *ImportHandler) writeHTTPError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{Error: &RPCError{
		Code:    code,
		Message: message,
	}}
	json.NewEncoder(w).Encode(resp)
}
