package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/message-db/message-db/internal/api"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store"
)

// mockStore is a minimal mock of the store for testing auth middleware
type mockStore struct {
	namespaces map[string]*mockNamespace
}

type mockNamespace struct {
	ID        string
	TokenHash string
}

func newMockStore() *mockStore {
	return &mockStore{
		namespaces: make(map[string]*mockNamespace),
	}
}

func (m *mockStore) GetNamespace(ctx context.Context, id string) (*store.Namespace, error) {
	ns, exists := m.namespaces[id]
	if !exists {
		return nil, store.ErrNamespaceNotFound
	}
	return &store.Namespace{
		ID:        ns.ID,
		TokenHash: ns.TokenHash,
	}, nil
}

func (m *mockStore) addNamespace(id, tokenHash string) {
	m.namespaces[id] = &mockNamespace{
		ID:        id,
		TokenHash: tokenHash,
	}
}

// Test MDB002_2A_T5: Test valid token allows request
func TestMDB002_2A_T5_ValidTokenAllowsRequest(t *testing.T) {
	// Create mock store with namespace
	ms := newMockStore()
	token, _ := auth.GenerateToken("test-ns")
	tokenHash := auth.HashToken(token)
	ms.addNamespace("test-ns", tokenHash)

	// Create test handler
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		
		// Verify namespace is in context
		namespace, ok := api.GetNamespaceFromContext(r.Context())
		if !ok {
			t.Error("Namespace not found in context")
		}
		if namespace != "test-ns" {
			t.Errorf("Expected namespace 'test-ns', got '%s'", namespace)
		}
		
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	authHandler := api.AuthMiddleware(ms, false)(handler)

	// Create request with valid token
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if !called {
		t.Error("Handler was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// Test MDB002_2A_T6: Test missing token returns AUTH_REQUIRED
func TestMDB002_2A_T6_MissingTokenReturnsAuthRequired(t *testing.T) {
	ms := newMockStore()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when token is missing")
	})

	authHandler := api.AuthMiddleware(ms, false)(handler)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
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

	if errResp.Error.Code != "AUTH_REQUIRED" {
		t.Errorf("Expected error code 'AUTH_REQUIRED', got '%s'", errResp.Error.Code)
	}
}

// Test MDB002_2A_T7: Test invalid token returns AUTH_INVALID_TOKEN
func TestMDB002_2A_T7_InvalidTokenReturnsAuthInvalidToken(t *testing.T) {
	ms := newMockStore()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when token is invalid")
	})

	authHandler := api.AuthMiddleware(ms, false)(handler)

	// Invalid token format
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	req.Header.Set("Authorization", "Bearer invalid_token_format")
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
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

	if errResp.Error.Code != "AUTH_INVALID_TOKEN" {
		t.Errorf("Expected error code 'AUTH_INVALID_TOKEN', got '%s'", errResp.Error.Code)
	}
}

// Test MDB002_2A_T8: Test wrong namespace token returns AUTH_UNAUTHORIZED
func TestMDB002_2A_T8_WrongNamespaceTokenReturnsUnauthorized(t *testing.T) {
	ms := newMockStore()
	
	// Create namespace with one token
	correctToken, _ := auth.GenerateToken("test-ns")
	correctHash := auth.HashToken(correctToken)
	ms.addNamespace("test-ns", correctHash)

	// Try to use a different token for the same namespace
	wrongToken, _ := auth.GenerateToken("test-ns")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when token doesn't match")
	})

	authHandler := api.AuthMiddleware(ms, false)(handler)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	req.Header.Set("Authorization", "Bearer "+wrongToken)
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
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

	if errResp.Error.Code != "AUTH_UNAUTHORIZED" {
		t.Errorf("Expected error code 'AUTH_UNAUTHORIZED', got '%s'", errResp.Error.Code)
	}
}

// Test MDB002_2A_T9: Test namespace added to context
func TestMDB002_2A_T9_NamespaceAddedToContext(t *testing.T) {
	ms := newMockStore()
	token, _ := auth.GenerateToken("my-namespace")
	tokenHash := auth.HashToken(token)
	ms.addNamespace("my-namespace", tokenHash)

	var capturedNamespace string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ns, ok := api.GetNamespaceFromContext(r.Context())
		if !ok {
			t.Error("Namespace not found in context")
		}
		capturedNamespace = ns
		w.WriteHeader(http.StatusOK)
	})

	authHandler := api.AuthMiddleware(ms, false)(handler)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if capturedNamespace != "my-namespace" {
		t.Errorf("Expected namespace 'my-namespace', got '%s'", capturedNamespace)
	}
}

// Test that Bearer scheme is required
func TestMDB002_2A_BearerSchemeRequired(t *testing.T) {
	ms := newMockStore()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	authHandler := api.AuthMiddleware(ms, false)(handler)

	// Try without Bearer scheme
	token, _ := auth.GenerateToken("test")
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	req.Header.Set("Authorization", token) // Missing "Bearer "
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(w.Body).Decode(&errResp)

	if errResp.Error.Code != "AUTH_REQUIRED" {
		t.Errorf("Expected error code 'AUTH_REQUIRED', got '%s'", errResp.Error.Code)
	}
}

// Test that test mode bypasses auth
func TestMDB002_2A_TestModeBypassesAuth(t *testing.T) {
	ms := newMockStore()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		
		// Verify test mode is set in context
		if !api.IsTestMode(r.Context()) {
			t.Error("Test mode should be set in context")
		}
		
		w.WriteHeader(http.StatusOK)
	})

	// Enable test mode
	authHandler := api.AuthMiddleware(ms, true)(handler)

	// No auth header
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`["sys.version"]`))
	w := httptest.NewRecorder()

	authHandler.ServeHTTP(w, req)

	if !called {
		t.Error("Handler should be called in test mode even without auth")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 in test mode, got %d", w.Code)
	}
}
