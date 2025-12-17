// Package main provides the Message DB HTTP server with RPC API.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/message-db/message-db/internal/api"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

const (
	version          = "1.3.0"
	defaultPort      = 8080
	defaultNamespace = "default"
	shutdownTimeout  = 10 * time.Second
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", defaultPort, "HTTP server port")
	testMode := flag.Bool("test-mode", false, "Run in test mode (in-memory SQLite)")
	defaultToken := flag.String("token", "", "Token for default namespace (if empty, one is generated)")
	flag.Parse()

	// Initialize SQLite store (in-memory for now)
	// NOTE: Must use file:xxx?mode=memory&cache=shared format to avoid conflicts
	// with namespace databases that also use shared-cache in-memory SQLite.
	// Using plain ":memory:" causes corruption when accessed concurrently with
	// file:xxx?mode=memory&cache=shared databases (modernc.org/sqlite driver issue).
	db, err := sql.Open("sqlite", "file:messagedb_metadata?mode=memory&cache=shared")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	// Ensure default namespace exists and get/create token
	token, err := ensureDefaultNamespace(context.Background(), st, *defaultToken)
	if err != nil {
		log.Fatalf("Failed to ensure default namespace: %v", err)
	}

	// Print default namespace token
	log.Printf("═══════════════════════════════════════════════════════")
	log.Printf("DEFAULT NAMESPACE TOKEN:")
	log.Printf("%s", token)
	log.Printf("═══════════════════════════════════════════════════════")

	// Create pubsub for real-time notifications
	pubsub := api.NewPubSub()

	// Create RPC handler
	rpcHandler := api.NewRPCHandler(version, st, pubsub)

	// Create SSE handler
	sseHandler := api.NewSSEHandler(st, pubsub, *testMode)

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
		fmt.Fprintf(w, `{"version":"%s"}`, version)
	})

	// RPC endpoint with auth middleware
	rpcWithAuth := api.AuthMiddleware(st, *testMode)(rpcHandler)
	mux.Handle("/rpc", api.LoggingMiddleware(rpcWithAuth))

	// SSE subscription endpoint
	mux.HandleFunc("/subscribe", sseHandler.HandleSubscribe)

	// Create server with proper timeouts and limits for high concurrency
	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Create TCP listener with custom configuration for high concurrency
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Message DB server starting on %s (version %s)", addr, version)
		serverErrors <- server.Serve(listener)
	}()

	// Wait for interrupt signal or server error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}

	case sig := <-shutdown:
		log.Printf("Shutdown signal received: %v", sig)

		// Create context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Graceful shutdown failed: %v", err)
			if err := server.Close(); err != nil {
				log.Printf("Force shutdown error: %v", err)
			}
		}

		log.Println("Server stopped")
	}
}

// ensureDefaultNamespace creates the default namespace if it doesn't exist.
// If providedToken is non-empty, it uses that token; otherwise generates one.
func ensureDefaultNamespace(ctx context.Context, st interface {
	GetNamespace(ctx context.Context, id string) (*store.Namespace, error)
	CreateNamespace(ctx context.Context, id, tokenHash, description string) error
}, providedToken string) (string, error) {
	var token string
	var err error

	// Use provided token or generate one
	if providedToken != "" {
		// Validate provided token format
		ns, err := auth.ParseToken(providedToken)
		if err != nil {
			return "", fmt.Errorf("invalid token format: %w", err)
		}
		if ns != defaultNamespace {
			return "", fmt.Errorf("provided token is for namespace '%s', expected '%s'", ns, defaultNamespace)
		}
		token = providedToken
	} else {
		token, err = auth.GenerateToken(defaultNamespace)
		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}
	}

	tokenHash := auth.HashToken(token)

	// Try to get existing namespace
	_, err = st.GetNamespace(ctx, defaultNamespace)
	if err != nil {
		// Namespace doesn't exist, create it
		err = st.CreateNamespace(ctx, defaultNamespace, tokenHash, "Default namespace")
		if err != nil {
			return "", fmt.Errorf("failed to create namespace: %w", err)
		}
		log.Printf("Created default namespace: %s", defaultNamespace)
	} else {
		log.Printf("Default namespace already exists: %s", defaultNamespace)
	}

	return token, nil
}
