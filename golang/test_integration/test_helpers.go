package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eventodb/eventodb/internal/api"
	"github.com/eventodb/eventodb/internal/auth"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/eventodb/eventodb/internal/store/pebble"
	"github.com/eventodb/eventodb/internal/store/postgres"
	"github.com/eventodb/eventodb/internal/store/sqlite"
	"github.com/google/uuid"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// BackendType represents the database backend to use for tests
type BackendType string

const (
	BackendSQLite   BackendType = "sqlite"
	BackendPostgres BackendType = "postgres"
	BackendPebble   BackendType = "pebble"
)

// GetTestBackend returns the backend type based on TEST_BACKEND environment variable.
// Defaults to "sqlite" if not set.
func GetTestBackend() BackendType {
	backend := os.Getenv("TEST_BACKEND")
	switch backend {
	case "postgres", "postgresql":
		return BackendPostgres
	case "pebble":
		return BackendPebble
	default:
		return BackendSQLite
	}
}

// GetAllTestBackends returns all available backends for comprehensive testing
func GetAllTestBackends() []BackendType {
	return []BackendType{BackendSQLite, BackendPostgres, BackendPebble}
}

// TestEnv holds the test environment configuration
type TestEnv struct {
	Store     store.Store
	Namespace string
	Token     string
	cleanup   func()
}

// Cleanup releases all resources
func (e *TestEnv) Cleanup() {
	if e.cleanup != nil {
		e.cleanup()
	}
}

// SetupTestEnv creates a test environment with the configured backend
func SetupTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	backend := GetTestBackend()
	return SetupTestEnvWithBackend(t, backend)
}

// SetupTestEnvWithBackend creates a test environment with a specific backend
func SetupTestEnvWithBackend(t *testing.T, backend BackendType) *TestEnv {
	t.Helper()

	switch backend {
	case BackendPostgres:
		return setupPostgresEnv(t)
	case BackendPebble:
		return setupPebbleEnv(t)
	default:
		return setupSQLiteEnv(t)
	}
}

// setupSQLiteEnv creates a SQLite-based test environment
func setupSQLiteEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Create unique in-memory database for this test
	dbName := fmt.Sprintf("file:test-%s-%s?mode=memory",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}

	// Create store with test mode
	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create SQLite store: %v", err)
	}

	// Create unique namespace for this test
	namespace := fmt.Sprintf("test-%s-%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Test namespace")
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Eagerly initialize the namespace database
	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
		_ = db.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// setupPostgresEnv creates a PostgreSQL-based test environment
func setupPostgresEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Get connection info from environment or use defaults
	host := getEnvDefault("POSTGRES_HOST", "localhost")
	port := getEnvDefault("POSTGRES_PORT", "5432")
	user := getEnvDefault("POSTGRES_USER", "postgres")
	password := getEnvDefault("POSTGRES_PASSWORD", "postgres")
	dbname := getEnvDefault("POSTGRES_DB", "postgres")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL connection: %v", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	// Create store
	st, err := postgres.New(db)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create PostgreSQL store: %v", err)
	}

	// Create unique namespace for this test
	namespace := fmt.Sprintf("test_ns_%s_%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Test namespace")
	if err != nil {
		st.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// setupPebbleEnv creates a Pebble-based test environment
func setupPebbleEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Create temporary directory for Pebble data (not used in memory mode but required)
	tmpDir := t.TempDir()

	// Create store with in-memory mode for fastest tests
	st, err := pebble.NewWithConfig(tmpDir, &pebble.Config{
		TestMode: true,
		InMemory: true, // Use in-memory for maximum test speed
	})
	if err != nil {
		t.Fatalf("Failed to create Pebble store: %v", err)
	}

	// Create unique namespace for this test
	namespace := fmt.Sprintf("test-%s-%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Test namespace")
	if err != nil {
		st.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Eagerly initialize the namespace database
	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// SetupTestEnvWithDefaultNamespace creates a test environment with "default" namespace
func SetupTestEnvWithDefaultNamespace(t *testing.T) *TestEnv {
	t.Helper()

	backend := GetTestBackend()
	switch backend {
	case BackendPostgres:
		return setupPostgresEnvWithDefaultNamespace(t)
	case BackendPebble:
		return setupPebbleEnvWithDefaultNamespace(t)
	default:
		return setupSQLiteEnvWithDefaultNamespace(t)
	}
}

// setupSQLiteEnvWithDefaultNamespace creates SQLite env with "default" namespace
func setupSQLiteEnvWithDefaultNamespace(t *testing.T) *TestEnv {
	t.Helper()

	dbName := fmt.Sprintf("file:test-%s-%s?mode=memory",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}

	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create SQLite store: %v", err)
	}

	namespace := "default"
	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Default namespace")
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
		_ = db.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// setupPostgresEnvWithDefaultNamespace creates PostgreSQL env with "default" namespace
func setupPostgresEnvWithDefaultNamespace(t *testing.T) *TestEnv {
	t.Helper()

	host := getEnvDefault("POSTGRES_HOST", "localhost")
	port := getEnvDefault("POSTGRES_PORT", "5432")
	user := getEnvDefault("POSTGRES_USER", "postgres")
	password := getEnvDefault("POSTGRES_PASSWORD", "postgres")
	dbname := getEnvDefault("POSTGRES_DB", "postgres")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL connection: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	st, err := postgres.New(db)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create PostgreSQL store: %v", err)
	}

	// Use a unique "default" namespace per test to avoid conflicts
	namespace := fmt.Sprintf("default_%s_%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Default namespace")
	if err != nil {
		st.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// setupPebbleEnvWithDefaultNamespace creates Pebble env with "default" namespace
func setupPebbleEnvWithDefaultNamespace(t *testing.T) *TestEnv {
	t.Helper()

	// Create temporary directory for Pebble data (not used in memory mode but required)
	tmpDir := t.TempDir()

	// Create store with in-memory mode for fastest tests
	st, err := pebble.NewWithConfig(tmpDir, &pebble.Config{
		TestMode: true,
		InMemory: true, // Use in-memory for maximum test speed
	})
	if err != nil {
		t.Fatalf("Failed to create Pebble store: %v", err)
	}

	// Use a unique "default" namespace per test to avoid conflicts
	namespace := fmt.Sprintf("default_%s_%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Default namespace")
	if err != nil {
		st.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Eagerly initialize the namespace database
	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
	}

	return &TestEnv{
		Store:     st,
		Namespace: namespace,
		Token:     token,
		cleanup:   cleanup,
	}
}

// TestServer holds the HTTP test server configuration
type TestServer struct {
	Port    int
	Token   string
	Env     *TestEnv
	cleanup func()
}

// Cleanup releases all resources
func (s *TestServer) Cleanup() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

// URL returns the base URL for the test server
func (s *TestServer) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port)
}

// SetupTestServer creates a test HTTP server with the configured backend
func SetupTestServer(t *testing.T) *TestServer {
	t.Helper()

	env := SetupTestEnvWithDefaultNamespace(t)

	// Create pubsub for real-time notifications
	pubsub := api.NewPubSub()

	// Create RPC handler
	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, pubsub)

	// Create SSE handler
	sseHandler := api.NewSSEHandler(env.Store, pubsub, true)

	// Create import handler
	importHandler := api.NewImportHandler(env.Store)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Version endpoint
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"version":"1.4.0"}`)
	})

	// RPC endpoint with auth middleware (test mode)
	rpcWithAuth := api.AuthMiddleware(env.Store, true)(rpcHandler)
	mux.Handle("/rpc", api.LoggingMiddleware(rpcWithAuth))

	// SSE subscription endpoint
	mux.HandleFunc("/subscribe", sseHandler.HandleSubscribe)

	// Import endpoint with auth middleware (test mode)
	importWithAuth := api.AuthMiddleware(env.Store, true)(importHandler)
	mux.Handle("/import", api.LoggingMiddleware(importWithAuth))

	// Start server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		env.Cleanup()
		t.Fatalf("Failed to create listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}

	go server.Serve(listener)
	time.Sleep(50 * time.Millisecond) // Give server time to start

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		listener.Close()
		env.Cleanup()
	}

	return &TestServer{
		Port:    port,
		Token:   env.Token,
		Env:     env,
		cleanup: cleanup,
	}
}

// SetupIsolatedTestServer creates a test HTTP server with a unique namespace,
// avoiding cross-test pollution on shared backends like PostgreSQL.
func SetupIsolatedTestServer(t *testing.T) *TestServer {
	t.Helper()

	env := SetupTestEnv(t) // unique namespace per test

	pubsub := api.NewPubSub()
	rpcHandler := api.NewRPCHandler("1.4.0", env.Store, pubsub)
	sseHandler := api.NewSSEHandler(env.Store, pubsub, true)
	importHandler := api.NewImportHandler(env.Store)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	rpcWithAuth := api.AuthMiddleware(env.Store, true)(rpcHandler)
	mux.Handle("/rpc", api.LoggingMiddleware(rpcWithAuth))
	mux.HandleFunc("/subscribe", sseHandler.HandleSubscribe)
	importWithAuth := api.AuthMiddleware(env.Store, true)(importHandler)
	mux.Handle("/import", api.LoggingMiddleware(importWithAuth))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		env.Cleanup()
		t.Fatalf("Failed to create listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	time.Sleep(50 * time.Millisecond)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		listener.Close()
		env.Cleanup()
	}

	return &TestServer{
		Port:    port,
		Token:   env.Token,
		Env:     env,
		cleanup: cleanup,
	}
}

// Helper functions

func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func sanitizeName(name string) string {
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result += string(r)
		} else if r == '/' || r == ' ' || r == '-' {
			result += "_"
		}
	}
	// Limit length for Postgres schema names
	if len(result) > 40 {
		result = result[:40]
	}
	return result
}
