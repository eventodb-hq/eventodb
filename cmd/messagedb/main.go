// Package main provides the Message DB HTTP server with RPC API.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
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
	version           = "1.3.0"
	defaultPort       = 8080
	defaultNamespace  = "default"
	shutdownTimeout   = 10 * time.Second
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", defaultPort, "HTTP server port")
	testMode := flag.Bool("test-mode", false, "Run in test mode (in-memory SQLite, auto-create namespaces)")
	flag.Parse()

	// Initialize SQLite store (in-memory for now)
	db, err := sql.Open("sqlite", ":memory:")
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
	defaultToken, err := ensureDefaultNamespace(context.Background(), st)
	if err != nil {
		log.Fatalf("Failed to ensure default namespace: %v", err)
	}

	// Print default namespace token
	log.Printf("═══════════════════════════════════════════════════════")
	log.Printf("DEFAULT NAMESPACE TOKEN:")
	log.Printf("%s", defaultToken)
	log.Printf("═══════════════════════════════════════════════════════")

	// Create RPC handler
	rpcHandler := api.NewRPCHandler(version, st)

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

	// Create server
	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Message DB server starting on %s (version %s)", addr, version)
		serverErrors <- server.ListenAndServe()
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

// ensureDefaultNamespace creates the default namespace if it doesn't exist
// and returns its token. If it already exists, generates a new token for it.
func ensureDefaultNamespace(ctx context.Context, st interface {
	GetNamespace(ctx context.Context, id string) (*store.Namespace, error)
	CreateNamespace(ctx context.Context, id, tokenHash, description string) error
}) (string, error) {
	// Try to get existing namespace
	_, err := st.GetNamespace(ctx, defaultNamespace)
	
	var token string
	
	if err != nil {
		// Namespace doesn't exist, create it
		token, err = auth.GenerateToken(defaultNamespace)
		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}
		
		tokenHash := auth.HashToken(token)
		
		err = st.CreateNamespace(ctx, defaultNamespace, tokenHash, "Default namespace")
		if err != nil {
			return "", fmt.Errorf("failed to create namespace: %w", err)
		}
		
		log.Printf("Created default namespace: %s", defaultNamespace)
	} else {
		// Namespace exists, generate a new token for it
		// Note: In production, you'd want to retrieve the existing token or use a different approach
		// For now, we'll generate a new one (this is for development/testing)
		token, err = auth.GenerateToken(defaultNamespace)
		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}
		log.Printf("Default namespace already exists: %s", defaultNamespace)
	}
	
	return token, nil
}
