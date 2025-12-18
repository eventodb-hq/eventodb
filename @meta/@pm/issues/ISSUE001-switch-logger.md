# ISSUE001: Migration Plan - Standard `log` Package → `zerolog`

**Status**: Planned  
**Priority**: Medium  
**Effort**: 5-9 hours  
**Created**: 2024-12-18

---

## **Current State Analysis**

### Files Using Standard `log` Package:
1. **`golang/cmd/messagedb/main.go`** (20 log statements)
   - Info logs: Server startup, connection status, namespace info
   - Fatal logs: Configuration and initialization errors
   - Special formatting: Token display with decorative borders

2. **`golang/internal/api/middleware.go`** (3 log statements)
   - HTTP request logging with method, path, status code, duration
   - Error-specific log for 5xx status codes

3. **`golang/internal/api/sse.go`** (2 log statements)
   - Error logs for message fetching failures

4. **`golang/internal/api/rpc.go`** (6 log statements)
   - Debug log: RPC method calls
   - Error logs: JSON parsing, encoding errors, 500 errors
   - Info log: Method invocation tracking

### Logging Patterns Identified:
- **Startup/Configuration**: Database connections, server start, namespace creation
- **Request Logging**: HTTP middleware with timing and status codes
- **Error Logging**: 500 errors, JSON encoding failures, database fetch errors
- **Debug Logging**: RPC method invocation
- **Fatal Errors**: Configuration validation, store creation failures

### No Logging in:
- `golang/internal/store/*` packages
- `golang/internal/auth/*` packages
- `golang/internal/migrate/*` packages
- Test files (they use `fmt.Printf` which is appropriate for tests)

---

## **Migration Strategy**

### Phase 1: Setup & Infrastructure
1. **Add zerolog dependency**
   - Update `go.mod` with `github.com/rs/zerolog`
   - Run `go get github.com/rs/zerolog`

2. **Create logger package/initialization**
   - Create `golang/internal/logger/logger.go` with:
     - Global logger initialization
     - Log level configuration (from env vars or flags)
     - Output format configuration (JSON for production, console for dev)
     - Context-aware logger helpers

3. **Add configuration options** to main.go:
   - `--log-level` flag (debug, info, warn, error)
   - `--log-format` flag (json, console)
   - Environment variable support (LOG_LEVEL, LOG_FORMAT)

### Phase 2: Migration by Component

#### 2.1 Main Application (`main.go`)
**Changes:**
- Initialize zerolog logger at startup
- Replace `log.Printf` with structured logging
- Replace `log.Fatalf` with `log.Fatal().Err(err).Msg()`
- Add structured fields: `dbType`, `address`, `version`, `namespace`
- Keep visual token display but log it properly

**Example transformation:**
```go
// Before
log.Printf("Connected to PostgreSQL database")

// After
log.Info().
    Str("db_type", "postgres").
    Str("connection", cfg.connStr).
    Msg("Connected to database")
```

#### 2.2 HTTP Middleware (`middleware.go`)
**Changes:**
- Add request-scoped logger to context
- Log with structured fields: method, path, status, duration, error (if 5xx)
- Optionally add request ID for tracing

**Example transformation:**
```go
// Before
log.Printf("%s %s %d %v [ERROR]", r.Method, r.URL.Path, wrapped.statusCode, duration)

// After
log.Error().
    Str("method", r.Method).
    Str("path", r.URL.Path).
    Int("status", wrapped.statusCode).
    Dur("duration", duration).
    Msg("HTTP request error")
```

#### 2.3 RPC Handler (`rpc.go`)
**Changes:**
- Add method name, request ID to logs
- Debug level for method invocation
- Error level with proper error wrapping
- Add RPC-specific fields: namespace, method, args_count

**Example transformation:**
```go
// Before
log.Printf("RPC: %s", method)

// After
log.Debug().
    Str("method", method).
    Str("namespace", namespace).
    Int("args_count", len(args)).
    Msg("RPC method invoked")
```

#### 2.4 SSE Handler (`sse.go`)
**Changes:**
- Add stream/category name to logs
- Include namespace and subscription details

**Example transformation:**
```go
// Before
log.Printf("Error fetching initial stream messages: %v", err)

// After
log.Error().
    Err(err).
    Str("stream", streamName).
    Str("namespace", namespace).
    Int64("position", startPosition).
    Msg("Failed to fetch initial stream messages")
```

### Phase 3: Enhanced Features

#### 3.1 Context-Aware Logging
- Add logger to request context in middleware
- Include trace ID, namespace in all logs
- Propagate logger through request lifecycle

#### 3.2 Log Levels & Sampling
- **Fatal**: Startup failures (DB connection, config errors)
- **Error**: 5xx errors, encoding failures, DB errors
- **Warn**: Auth failures, validation errors
- **Info**: Server start, namespace operations, connections
- **Debug**: RPC method calls, SSE subscriptions
- **Trace**: Detailed request/response data (optional)

#### 3.3 Structured Fields Strategy
- **Common fields**: `version`, `namespace`, `db_type`
- **HTTP fields**: `method`, `path`, `status`, `duration`, `request_id`
- **RPC fields**: `rpc_method`, `args_count`, `namespace`
- **Error fields**: `error`, `error_code`, `details`

### Phase 4: Testing & Validation

1. **Test mode consideration**
   - Keep console format for tests
   - Allow disabling logs in test mode
   - Maintain existing test behavior

2. **Integration testing**
   - Verify all log levels work
   - Check JSON output format
   - Test context propagation

3. **Performance testing**
   - Benchmark logging performance
   - Ensure no degradation under high load

---

## **Implementation Steps (Ordered)**

1. ⬜ Add `github.com/rs/zerolog` to `go.mod`
2. ⬜ Create `golang/internal/logger/logger.go` with initialization logic
3. ⬜ Update `main.go`:
   - Add logger initialization
   - Add CLI flags for log config
   - Migrate all log statements
4. ⬜ Update `middleware.go`:
   - Add request-scoped logger to context
   - Migrate logging middleware
5. ⬜ Update `rpc.go`:
   - Use context logger
   - Migrate all log statements
6. ⬜ Update `sse.go`:
   - Use context logger
   - Migrate all log statements
7. ⬜ Update `handlers.go` (if any logging exists)
8. ⬜ Remove `import "log"` from all files
9. ⬜ Run tests to ensure nothing broke
10. ⬜ Update documentation/README

---

## **Proposed Logger Package Structure**

```go
// golang/internal/logger/logger.go
package logger

import (
    "context"
    "io"
    "os"
    "github.com/rs/zerolog"
)

type contextKey string

const loggerKey contextKey = "logger"

// Initialize sets up the global logger
func Initialize(level string, format string) {
    // Configure output
    var output io.Writer = os.Stdout
    if format == "console" {
        output = zerolog.ConsoleWriter{Out: os.Stdout}
    }
    
    // Parse level
    logLevel := zerolog.InfoLevel
    switch level {
    case "debug": logLevel = zerolog.DebugLevel
    case "info": logLevel = zerolog.InfoLevel
    case "warn": logLevel = zerolog.WarnLevel
    case "error": logLevel = zerolog.ErrorLevel
    }
    
    zerolog.SetGlobalLevel(logLevel)
    logger := zerolog.New(output).With().Timestamp().Logger()
    zerolog.DefaultContextLogger = &logger
}

// FromContext retrieves logger from context
func FromContext(ctx context.Context) *zerolog.Logger {
    if logger, ok := ctx.Value(loggerKey).(*zerolog.Logger); ok {
        return logger
    }
    logger := zerolog.Ctx(ctx)
    return logger
}

// WithContext adds logger to context
func WithContext(ctx context.Context, logger *zerolog.Logger) context.Context {
    return context.WithValue(ctx, loggerKey, logger)
}
```

---

## **Benefits of This Migration**

1. **Structured Logging**: JSON output for production, easy parsing by log aggregators
2. **Performance**: zerolog is zero-allocation and faster than standard log
3. **Context Propagation**: Request-scoped loggers with trace IDs
4. **Flexible Configuration**: Runtime log level control
5. **Better Debugging**: Structured fields make troubleshooting easier
6. **Production Ready**: JSON format works with ELK, Splunk, Datadog, etc.

---

## **Risks & Mitigations**

| Risk | Mitigation |
|------|------------|
| Breaking changes in log output | Keep console format for development/testing |
| Performance impact | zerolog is faster than standard log, should improve |
| Test failures | Maintain test mode behavior, use same log levels |
| Missing logs | Comprehensive review of all log statements |

---

## **Estimated Effort**

- **Phase 1**: 1-2 hours (setup, logger package)
- **Phase 2**: 2-3 hours (migrate all files)
- **Phase 3**: 1-2 hours (enhanced features, context propagation)
- **Phase 4**: 1-2 hours (testing, validation)
- **Total**: ~5-9 hours

---

## **Files to Modify**

1. `golang/go.mod` - Add zerolog dependency
2. `golang/internal/logger/logger.go` - New file (logger package)
3. `golang/cmd/messagedb/main.go` - Migrate 20 log statements
4. `golang/internal/api/middleware.go` - Migrate 3 log statements
5. `golang/internal/api/sse.go` - Migrate 2 log statements
6. `golang/internal/api/rpc.go` - Migrate 6 log statements

**Total**: 31 log statements across 4 files + 2 new/modified files

---

## **Notes**

- All logging is currently concentrated in the HTTP/API layer and main application
- Internal packages (store, auth, migrate) have no logging - good separation of concerns
- Test files appropriately use `fmt.Printf` and don't need migration
- The migration should be done in a single PR to avoid mixed logging approaches

---

**References**:
- zerolog GitHub: https://github.com/rs/zerolog
- zerolog Documentation: https://github.com/rs/zerolog#readme
