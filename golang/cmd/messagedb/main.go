// Package main provides the Message DB HTTP server with RPC API using fasthttp.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/message-db/message-db/internal/api"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/logger"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/postgres"
	"github.com/message-db/message-db/internal/store/sqlite"
	"github.com/message-db/message-db/internal/store/timescale"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

const (
	version          = "1.4.0"
	defaultPort      = 8080
	defaultNamespace = "default"
	shutdownTimeout  = 10 * time.Second
)

// Database configuration
type dbConfig struct {
	dbType   string // "postgres", "timescale", or "sqlite"
	connStr  string // Connection string for the database
	dataDir  string // Data directory for SQLite namespace databases
	testMode bool   // In-memory mode for testing
}

// parseDBConfig parses the database URL and returns configuration
func parseDBConfig(dbURL, dataDir, dbTypeOverride string, testMode bool) (*dbConfig, error) {
	// Test mode: use in-memory SQLite
	if testMode {
		return &dbConfig{
			dbType:   "sqlite",
			connStr:  "file:messagedb_metadata?mode=memory&cache=shared",
			dataDir:  "", // Not used in test mode
			testMode: true,
		}, nil
	}

	// No URL provided: error
	if dbURL == "" {
		return nil, fmt.Errorf("--db-url is required (use --test-mode for in-memory testing)")
	}

	// Parse the URL
	u, err := url.Parse(dbURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	switch u.Scheme {
	case "postgres", "postgresql":
		// Check for explicit TimescaleDB override
		dbType := "postgres"
		if dbTypeOverride == "timescale" {
			dbType = "timescale"
		}

		return &dbConfig{
			dbType:   dbType,
			connStr:  dbURL,
			dataDir:  "", // Not used for Postgres/TimescaleDB
			testMode: false,
		}, nil

	case "sqlite":
		// SQLite: extract the database filename from the URL
		// Format: sqlite://filename.db or sqlite:///path/to/filename.db
		dbFile := strings.TrimPrefix(dbURL, "sqlite://")
		if dbFile == "" {
			dbFile = "metadata.db"
		}

		// Require data-dir for SQLite
		if dataDir == "" {
			return nil, fmt.Errorf("--data-dir is required for SQLite")
		}

		// Ensure data directory exists
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}

		// Build full path to metadata database
		metadataPath := filepath.Join(dataDir, dbFile)

		return &dbConfig{
			dbType:   "sqlite",
			connStr:  metadataPath,
			dataDir:  dataDir,
			testMode: false,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported database scheme: %s (use postgres:// or sqlite://)", u.Scheme)
	}
}

// createStore creates the appropriate store based on configuration
func createStore(cfg *dbConfig) (store.Store, func(), error) {
	switch cfg.dbType {
	case "postgres":
		db, err := sql.Open("postgres", cfg.connStr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
		}

		// Configure connection pool for production
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		// Verify connection
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}

		st, err := postgres.New(db)
		if err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("failed to create PostgreSQL store: %w", err)
		}

		logger.Get().Info().
			Str("db_type", "postgres").
			Msg("Connected to PostgreSQL database")
		cleanup := func() {
			st.Close()
		}
		return st, cleanup, nil

	case "timescale":
		db, err := sql.Open("postgres", cfg.connStr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open TimescaleDB connection: %w", err)
		}

		// Configure connection pool for production
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		// Verify connection
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("failed to connect to TimescaleDB: %w", err)
		}

		st, err := timescale.New(db)
		if err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("failed to create TimescaleDB store: %w", err)
		}

		logger.Get().Info().
			Str("db_type", "timescale").
			Msg("Connected to TimescaleDB database")
		cleanup := func() {
			st.Close()
		}
		return st, cleanup, nil

	case "sqlite":
		var db *sql.DB
		var err error

		if cfg.testMode {
			// In-memory mode with shared cache
			db, err = sql.Open("sqlite", cfg.connStr)
		} else {
			// File-based SQLite with WAL mode
			dsn := cfg.connStr + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
			db, err = sql.Open("sqlite", dsn)
		}

		if err != nil {
			return nil, nil, fmt.Errorf("failed to open SQLite connection: %w", err)
		}

		st, err := sqlite.New(db, &sqlite.Config{
			TestMode: cfg.testMode,
			DataDir:  cfg.dataDir,
		})
		if err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("failed to create SQLite store: %w", err)
		}

		if cfg.testMode {
			logger.Get().Info().
				Str("db_type", "sqlite").
				Bool("test_mode", true).
				Msg("Using in-memory SQLite")
		} else {
			logger.Get().Info().
				Str("db_type", "sqlite").
				Str("path", cfg.connStr).
				Msg("Connected to SQLite database")
		}

		cleanup := func() {
			st.Close()
		}
		return st, cleanup, nil

	default:
		return nil, nil, fmt.Errorf("unknown database type: %s", cfg.dbType)
	}
}

func main() {
	// Parse command-line flags
	port := flag.Int("port", defaultPort, "HTTP server port")
	testMode := flag.Bool("test-mode", false, "Run in test mode (in-memory SQLite)")
	defaultToken := flag.String("token", "", "Token for default namespace (if empty, one is generated)")
	dbURL := flag.String("db-url", "", "Database URL (postgres://... or sqlite://filename.db)")
	dataDir := flag.String("data-dir", "", "Data directory for SQLite namespace databases (required for sqlite)")
	dbType := flag.String("db-type", "", "Database type override (use 'timescale' for TimescaleDB with postgres:// URL)")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", "console", "Log format (json, console)")
	flag.Parse()

	// Initialize logger
	logger.Initialize(*logLevel, *logFormat)

	// Parse database configuration
	cfg, err := parseDBConfig(*dbURL, *dataDir, *dbType, *testMode)
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Invalid database configuration")
	}

	// Initialize store based on database type
	st, cleanup, err := createStore(cfg)
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Failed to create store")
	}
	defer cleanup()

	// Ensure default namespace exists and get/create token
	token, err := ensureDefaultNamespace(context.Background(), st, *defaultToken)
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Failed to ensure default namespace")
	}

	// Print default namespace token
	logger.Get().Info().Msg("═══════════════════════════════════════════════════════")
	logger.Get().Info().Msg("DEFAULT NAMESPACE TOKEN:")
	logger.Get().Info().Msgf("%s", token)
	logger.Get().Info().Msg("═══════════════════════════════════════════════════════")

	// Create pubsub for real-time notifications
	pubsub := api.NewPubSub()

	// Create RPC handler
	rpcHandler := api.NewRPCHandler(version, st, pubsub)

	// Create SSE handler
	sseHandler := api.NewSSEHandler(st, pubsub, cfg.testMode)

	// Create fasthttp middleware
	authMiddlewareFast := api.AuthMiddlewareFast(st, cfg.testMode)

	// Create wrapped RPC handler with auth and logging for fasthttp
	rpcHandlerFast := api.FastHTTPRPCHandler(rpcHandler, cfg.testMode)
	rpcWithAuthFast := authMiddlewareFast(rpcHandlerFast)
	rpcWithLoggingFast := api.LoggingMiddlewareFast(rpcWithAuthFast)

	// Create SSE handler wrapper with auth
	sseHandlerFast := api.FastHTTPSSEHandler(sseHandler, cfg.testMode)
	sseWithAuthFast := authMiddlewareFast(sseHandlerFast)
	sseWithLoggingFast := api.LoggingMiddlewareFast(sseWithAuthFast)

	// Set up fasthttp router
	requestHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())

		switch path {
		case "/health":
			ctx.SetContentType("application/json")
			ctx.SetStatusCode(fasthttp.StatusOK)
			fmt.Fprintf(ctx, `{"status":"ok"}`)

		case "/version":
			ctx.SetContentType("application/json")
			ctx.SetStatusCode(fasthttp.StatusOK)
			fmt.Fprintf(ctx, `{"version":"%s"}`, version)

		case "/rpc":
			// Use native fasthttp handler with auth and logging
			rpcWithLoggingFast(ctx)

		case "/subscribe":
			// SSE handler with auth and logging
			sseWithLoggingFast(ctx)

		default:
			// Handle all pprof endpoints with a prefix check
			if len(path) >= 13 && path[:13] == "/debug/pprof/" {
				pprofhandler.PprofHandler(ctx)
				return
			}

			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetContentType("application/json")
			fmt.Fprintf(ctx, `{"error":{"code":"NOT_FOUND","message":"Endpoint not found"}}`)
		}
	}

	logger.Get().Info().Msg("pprof profiling endpoints enabled at /debug/pprof/")

	// Create fasthttp server with optimized settings
	addr := fmt.Sprintf(":%d", *port)
	server := &fasthttp.Server{
		Handler:                       requestHandler,
		Name:                          "MessageDB/" + version,
		ReadTimeout:                   30 * time.Second,
		WriteTimeout:                  30 * time.Second,
		IdleTimeout:                   120 * time.Second,
		MaxRequestBodySize:            4 * 1024 * 1024, // 4 MB
		Concurrency:                   256 * 1024,      // Handle up to 256K concurrent connections
		DisableKeepalive:              false,
		TCPKeepalive:                  true,
		TCPKeepalivePeriod:            30 * time.Second,
		MaxConnsPerIP:                 0, // No limit
		MaxRequestsPerConn:            0, // No limit
		ReduceMemoryUsage:             false,
		GetOnly:                       false,
		DisableHeaderNamesNormalizing: false,
	}

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Get().Info().
			Str("address", addr).
			Str("version", version).
			Str("engine", "fasthttp").
			Msg("Message DB server starting")
		serverErrors <- server.ListenAndServe(addr)
	}()

	// Wait for interrupt signal or server error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil {
			logger.Get().Fatal().Err(err).Msg("Server error")
		}

	case sig := <-shutdown:
		logger.Get().Info().Str("signal", sig.String()).Msg("Shutdown signal received")

		// Attempt graceful shutdown
		if err := server.Shutdown(); err != nil {
			logger.Get().Error().Err(err).Msg("Graceful shutdown failed")
		}

		logger.Get().Info().Msg("Server stopped")
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
		logger.Get().Info().Str("namespace", defaultNamespace).Msg("Created default namespace")
	} else {
		logger.Get().Info().Str("namespace", defaultNamespace).Msg("Default namespace already exists")
	}

	return token, nil
}
