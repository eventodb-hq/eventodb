// Package main provides the EventoDB HTTP server with RPC API using fasthttp.
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eventodb/eventodb/internal/api"
	"github.com/eventodb/eventodb/internal/auth"
	"github.com/eventodb/eventodb/internal/logger"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/eventodb/eventodb/internal/store/pebble"
	"github.com/eventodb/eventodb/internal/store/postgres"
	"github.com/eventodb/eventodb/internal/store/sqlite"
	"github.com/eventodb/eventodb/internal/store/timescale"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
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
	// Test mode without explicit db-url: use in-memory SQLite
	if testMode && dbURL == "" {
		return &dbConfig{
			dbType:   "sqlite",
			connStr:  "file:eventodb_metadata?mode=memory&cache=shared",
			dataDir:  "", // Not used in test mode
			testMode: true,
		}, nil
	}

	// No URL provided and not in test mode: error
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

	case "pebble":
		// Pebble: extract the data directory from the URL
		// Format: pebble:///path/to/data or pebble://path/to/data
		pebbleDir := strings.TrimPrefix(dbURL, "pebble://")
		if pebbleDir == "" {
			pebbleDir = "./data/pebble"
		}

		// In test mode, don't create directory (will use in-memory)
		if !testMode {
			// Ensure data directory exists
			if err := os.MkdirAll(pebbleDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create Pebble data directory: %w", err)
			}
		}

		return &dbConfig{
			dbType:   "pebble",
			connStr:  "", // Not used for Pebble
			dataDir:  pebbleDir,
			testMode: testMode,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported database scheme: %s (use postgres://, sqlite://, or pebble://)", u.Scheme)
	}
}

// createStore creates the appropriate store based on configuration
func createStore(cfg *dbConfig) (store.Store, func(), error) {
	switch cfg.dbType {
	case "postgres":
		db, err := sql.Open("pgx", cfg.connStr)
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
		db, err := sql.Open("pgx", cfg.connStr)
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

	case "pebble":
		st, err := pebble.NewWithConfig(cfg.dataDir, &pebble.Config{
			TestMode: cfg.testMode,
			InMemory: cfg.testMode, // Use in-memory when in test mode
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Pebble store: %w", err)
		}

		if cfg.testMode {
			logger.Get().Info().
				Str("db_type", "pebble").
				Bool("in_memory", true).
				Msg("Using in-memory Pebble")
		} else {
			logger.Get().Info().
				Str("db_type", "pebble").
				Str("path", cfg.dataDir).
				Msg("Connected to Pebble database")
		}

		cleanup := func() {
			st.Close()
		}
		return st, cleanup, nil

	default:
		return nil, nil, fmt.Errorf("unknown database type: %s", cfg.dbType)
	}
}

const banner = `
╔══════════════════════════════════════════════════════════════════════════════╗
║                                                                              ║
║   ███████╗██╗   ██╗███████╗███╗   ██╗████████╗ ██████╗ ██████╗ ██████╗       ║
║   ██╔════╝██║   ██║██╔════╝████╗  ██║╚══██╔══╝██╔═══██╗██╔══██╗██╔══██╗      ║
║   █████╗  ██║   ██║█████╗  ██╔██╗ ██║   ██║   ██║   ██║██║  ██║██████╔╝      ║
║   ██╔══╝  ╚██╗ ██╔╝██╔══╝  ██║╚██╗██║   ██║   ██║   ██║██║  ██║██╔══██╗      ║
║   ███████╗ ╚████╔╝ ███████╗██║ ╚████║   ██║   ╚██████╔╝██████╔╝██████╔╝      ║
║   ╚══════╝  ╚═══╝  ╚══════╝╚═╝  ╚═══╝   ╚═╝    ╚═════╝ ╚═════╝ ╚═════╝       ║
║                                                                              ║
║   High-performance event store for Event Sourcing & Pub/Sub                  ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
`

const helpText = `
USAGE:
    eventodb [OPTIONS]
    eventodb [COMMAND]

COMMANDS:
    version, -v, --version    Show version information
    help, -h, --help          Show this help message

OPTIONS:
    -port <port>              HTTP server port (default: 8080)
                              Env: EVENTODB_PORT

    -db-url <url>             Database connection URL (required unless --test-mode)
                              Formats:
                                postgres://user:pass@host:5432/dbname
                                sqlite://filename.db
                                pebble:///path/to/data (or pebble://memory)
                              Env: EVENTODB_DB_URL

    -data-dir <path>          Data directory for SQLite namespace databases
                              Required when using sqlite:// URL
                              Env: EVENTODB_DATA_DIR

    -db-type <type>           Database type override
                              Use 'timescale' with postgres:// URL for TimescaleDB
                              Env: EVENTODB_DB_TYPE

    -token <token>            Token for default namespace
                              If empty, one is auto-generated
                              Env: EVENTODB_TOKEN

    -test-mode                Run in test mode with in-memory SQLite
                              Auth is optional, namespaces auto-created
                              Env: EVENTODB_TEST_MODE

    -log-level <level>        Log level: debug, info, warn, error (default: info)
                              Env: EVENTODB_LOG_LEVEL

    -log-format <format>      Log format: json, console (default: console)
                              Env: EVENTODB_LOG_FORMAT

EXAMPLES:
    # Development (in-memory)
    eventodb --test-mode --port 8080

    # SQLite (persistent)
    eventodb --db-url sqlite://eventodb.db --data-dir ./data

    # PostgreSQL
    eventodb --db-url postgres://user:pass@localhost:5432/eventodb

    # Pebble KV (persistent)
    eventodb --db-url pebble:///var/lib/eventodb/data

ENDPOINTS:
    POST /rpc                 JSON-RPC API endpoint
    GET  /subscribe           SSE subscription endpoint
    GET  /health              Health check
    GET  /version             Version info

DOCUMENTATION:
    https://github.com/eventodb-hq/eventodb
`

func printHelp() {
	fmt.Print(banner)
	fmt.Print(helpText)
}

func printVersion(full bool) {
	if full {
		fmt.Print(banner)
		fmt.Printf("Version:  %s\n", version)
		fmt.Printf("Commit:   %s\n", commit)
		fmt.Printf("Built:    %s\n", date)
	} else {
		fmt.Println(version)
	}
}

func main() {
	// No arguments: show help
	if len(os.Args) == 1 {
		printHelp()
		return
	}

	// Handle commands before flag parsing
	switch os.Args[1] {
	case "version", "--version", "-v":
		full := len(os.Args) > 2 && (os.Args[2] == "--full" || os.Args[2] == "-f")
		printVersion(full)
		return
	case "help", "--help", "-h":
		printHelp()
		return
	}

	// Custom usage function
	flag.Usage = printHelp

	// Parse command-line flags (with environment variable fallbacks)
	port := flag.Int("port", getEnvInt("EVENTODB_PORT", defaultPort), "")
	testMode := flag.Bool("test-mode", getEnvBool("EVENTODB_TEST_MODE", false), "")
	defaultToken := flag.String("token", getEnv("EVENTODB_TOKEN", ""), "")
	dbURL := flag.String("db-url", getEnv("EVENTODB_DB_URL", ""), "")
	dataDir := flag.String("data-dir", getEnv("EVENTODB_DATA_DIR", ""), "")
	dbType := flag.String("db-type", getEnv("EVENTODB_DB_TYPE", ""), "")
	logLevel := flag.String("log-level", getEnv("EVENTODB_LOG_LEVEL", "info"), "")
	logFormat := flag.String("log-format", getEnv("EVENTODB_LOG_FORMAT", "console"), "")
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
		Name:                          "EventoDB/" + version,
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
			Msg("EventoDB server starting")
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

// Environment variable helpers
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
