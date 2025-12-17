package unit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/message-db/message-db/internal/api"
)

// Test MDB002_1A_T5: Test valid RPC request parsed correctly
func TestMDB002_1A_T5_ValidRPCRequest(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Valid request: ["sys.version"]
	reqBody := `["sys.version"]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result != "1.3.0" {
		t.Errorf("Expected version '1.3.0', got '%s'", result)
	}
}

// Test MDB002_1A_T6: Test invalid JSON returns INVALID_REQUEST
func TestMDB002_1A_T6_InvalidJSON(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Invalid JSON
	reqBody := `{"invalid": json}`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Error.Code)
	}
}

// Test MDB002_1A_T7: Test missing method returns INVALID_REQUEST
func TestMDB002_1A_T7_MissingMethod(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Empty array
	reqBody := `[]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Error.Code)
	}

	if errResp.Error.Message != "Missing method name" {
		t.Errorf("Expected message 'Missing method name', got '%s'", errResp.Error.Message)
	}
}

// Test MDB002_1A_T8: Test unknown method returns METHOD_NOT_FOUND
func TestMDB002_1A_T8_UnknownMethod(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Unknown method
	reqBody := `["unknown.method"]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Code != "METHOD_NOT_FOUND" {
		t.Errorf("Expected error code 'METHOD_NOT_FOUND', got '%s'", errResp.Error.Code)
	}
}

// Test MDB002_1A_T9: Test success response format correct
func TestMDB002_1A_T9_SuccessResponseFormat(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Call sys.health which returns an object
	reqBody := `["sys.health"]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check Content-Type header
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Verify response is valid JSON
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify expected fields
	if result["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", result["status"])
	}
}

// Test MDB002_1A_T10: Test error response format correct
func TestMDB002_1A_T10_ErrorResponseFormat(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Trigger an error with unknown method
	reqBody := `["nonexistent.method"]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check Content-Type header
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Verify error response structure
	var errResp struct {
		Error struct {
			Code    string                 `json:"code"`
			Message string                 `json:"message"`
			Details map[string]interface{} `json:"details,omitempty"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	// Verify error object exists and has required fields
	if errResp.Error.Code == "" {
		t.Error("Error code is empty")
	}
	if errResp.Error.Message == "" {
		t.Error("Error message is empty")
	}
}

// Test that method must be a string
func TestMDB002_1A_MethodMustBeString(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Method is a number instead of string
	reqBody := `[123, "arg1"]`
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Error.Code)
	}
	if errResp.Error.Message != "Method name must be a string" {
		t.Errorf("Expected message 'Method name must be a string', got '%s'", errResp.Error.Message)
	}
}

// Test that only POST is allowed
func TestMDB002_1A_OnlyPostAllowed(t *testing.T) {
	handler := api.NewRPCHandler("1.3.0")

	// Try GET request
	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("Expected error code 'INVALID_REQUEST', got '%s'", errResp.Error.Code)
	}
}
