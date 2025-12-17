# EPIC MDB002: RPC API & Authentication Implementation Plan

## Test-Driven Development Approach

### Phase 1: HTTP Server & RPC Foundation (Day 1-3)

#### Phase MDB002_1A: CODE: Server Setup and RPC Handler
- [ ] Create `cmd/messagedb/main.go` - server binary entry point
- [ ] Set up HTTP server with port configuration
- [ ] Add health check endpoint `GET /health`
- [ ] Add version endpoint `GET /version`
- [ ] Implement graceful shutdown
- [ ] Add basic logging middleware
- [ ] Create `internal/api/rpc.go` - RPC handler
- [ ] Implement RPC request parser (array format)
- [ ] Add method routing logic (method name -> handler)
- [ ] Add request validation (method + args count)
- [ ] Implement response formatter (success + error)
- [ ] Add error response structure

#### Phase MDB002_1A: TESTS: Server and RPC Tests
- [ ] **MDB002_1A_T1: Test server starts and listens on configured port**
- [ ] **MDB002_1A_T2: Test health check returns 200 OK**
- [ ] **MDB002_1A_T3: Test version endpoint returns correct version**
- [ ] **MDB002_1A_T4: Test graceful shutdown closes connections**
- [ ] **MDB002_1A_T5: Test valid RPC request parsed correctly**
- [ ] **MDB002_1A_T6: Test invalid JSON returns INVALID_REQUEST**
- [ ] **MDB002_1A_T7: Test missing method returns INVALID_REQUEST**
- [ ] **MDB002_1A_T8: Test unknown method returns METHOD_NOT_FOUND**
- [ ] **MDB002_1A_T9: Test success response format correct**
- [ ] **MDB002_1A_T10: Test error response format correct**

### Phase 2: Authentication & Default Namespace (Day 4-6)

#### Phase MDB002_2A: CODE: Token System and Auth Middleware
- [ ] Create `internal/auth/token.go` - token functions
- [ ] Implement `GenerateToken(namespace string) (string, error)`
- [ ] Implement `ParseToken(token string) (namespace string, error)`
- [ ] Implement `HashToken(token string) string` using SHA-256
- [ ] Add token format validation
- [ ] Create `internal/api/middleware.go` - auth middleware
- [ ] Extract token from Authorization header
- [ ] Validate token format and query namespace from database
- [ ] Add namespace to request context
- [ ] Handle missing/invalid token errors
- [ ] Support test mode (optional auth)
- [ ] Implement `ensureDefaultNamespace()` in main.go
- [ ] Generate and print default namespace token on startup

#### Phase MDB002_2A: TESTS: Auth and Token Tests
- [ ] **MDB002_2A_T1: Test token generation creates valid format**
- [ ] **MDB002_2A_T2: Test token parsing extracts correct namespace**
- [ ] **MDB002_2A_T3: Test token hash is deterministic**
- [ ] **MDB002_2A_T4: Test invalid token format returns error**
- [ ] **MDB002_2A_T5: Test valid token allows request**
- [ ] **MDB002_2A_T6: Test missing token returns AUTH_REQUIRED**
- [ ] **MDB002_2A_T7: Test invalid token returns AUTH_INVALID_TOKEN**
- [ ] **MDB002_2A_T8: Test wrong namespace token returns AUTH_UNAUTHORIZED**
- [ ] **MDB002_2A_T9: Test namespace added to context**
- [ ] **MDB002_2A_T10: Test default namespace created on first run**
- [ ] **MDB002_2A_T11: Test default token printed to stdout**

### Phase 3: Stream Operations (Day 7-10)

#### Phase MDB002_3A: CODE: Stream RPC Methods
- [ ] Create `internal/api/handlers.go` - RPC method implementations
- [ ] Implement `handleStreamWrite(ctx, args)` - write messages
- [ ] Parse streamName, msg, opts arguments
- [ ] Generate message ID if not provided
- [ ] Handle optimistic locking (expectedVersion)
- [ ] Implement `handleStreamGet(ctx, args)` - read messages
- [ ] Handle position vs globalPosition filtering
- [ ] Apply batchSize limit
- [ ] Format response array
- [ ] Implement `handleStreamLast(ctx, args)` - get last message
- [ ] Support type filtering
- [ ] Implement `handleStreamVersion(ctx, args)` - get stream version
- [ ] Return stream version or null

#### Phase MDB002_3A: TESTS: Stream Operation Tests
- [ ] **MDB002_3A_T1: Test write message returns position**
- [ ] **MDB002_3A_T2: Test write with auto-generated ID**
- [ ] **MDB002_3A_T3: Test write with expectedVersion succeeds**
- [ ] **MDB002_3A_T4: Test write with wrong expectedVersion fails**
- [ ] **MDB002_3A_T5: Test write with metadata**
- [ ] **MDB002_3A_T6: Test namespace isolation (different tokens)**
- [ ] **MDB002_3A_T7: Test get stream returns messages**
- [ ] **MDB002_3A_T8: Test get with position filter**
- [ ] **MDB002_3A_T9: Test get with batchSize limit**
- [ ] **MDB002_3A_T10: Test last message returns latest**
- [ ] **MDB002_3A_T11: Test last with type filter**
- [ ] **MDB002_3A_T12: Test version returns correct number**

### Phase 4: Category Operations (Day 11-13)

#### Phase MDB002_4A: CODE: Category RPC Methods
- [ ] Implement `handleCategoryGet(ctx, args)` in handlers.go
- [ ] Parse categoryName and opts arguments
- [ ] Extract namespace from context
- [ ] Call store.GetCategoryMessages()
- [ ] Implement consumer group filtering
- [ ] Support correlation filtering
- [ ] Format response (include streamName in each message)

#### Phase MDB002_4A: TESTS: Category Operation Tests
- [ ] **MDB002_4A_T1: Test get category returns messages from multiple streams**
- [ ] **MDB002_4A_T2: Test category includes stream names in response**
- [ ] **MDB002_4A_T3: Test category with position filter**
- [ ] **MDB002_4A_T4: Test category with batchSize limit**
- [ ] **MDB002_4A_T5: Test category with consumer group (member 0 of 2)**
- [ ] **MDB002_4A_T6: Test consumer groups have no overlap**
- [ ] **MDB002_4A_T7: Test correlation filtering**
- [ ] **MDB002_4A_T8: Test empty category returns empty array**

### Phase 5: Namespace Management (Day 14-16)

#### Phase MDB002_5A: CODE: Namespace RPC Methods
- [ ] Implement `handleNamespaceCreate(ctx, args)` in handlers.go
- [ ] Generate namespace token and hash
- [ ] Create Postgres schema OR SQLite database
- [ ] Run namespace migrations
- [ ] Insert record into namespaces table
- [ ] Return namespace info with token
- [ ] Implement `handleNamespaceDelete(ctx, args)`
- [ ] Verify token matches namespace
- [ ] Drop Postgres schema OR delete SQLite database
- [ ] Delete from namespaces table
- [ ] Implement `handleNamespaceList(ctx, args)`
- [ ] Require admin token
- [ ] Query all namespaces with message counts
- [ ] Implement `handleNamespaceInfo(ctx, args)`
- [ ] Return namespace stats

#### Phase MDB002_5A: TESTS: Namespace Management Tests
- [ ] **MDB002_5A_T1: Test create namespace returns token**
- [ ] **MDB002_5A_T2: Test namespace schema/database created**
- [ ] **MDB002_5A_T3: Test duplicate namespace returns NAMESPACE_EXISTS**
- [ ] **MDB002_5A_T4: Test delete namespace removes all data**
- [ ] **MDB002_5A_T5: Test delete invalidates token**
- [ ] **MDB002_5A_T6: Test delete with wrong token fails**
- [ ] **MDB002_5A_T7: Test list returns all namespaces**
- [ ] **MDB002_5A_T8: Test list includes message counts**
- [ ] **MDB002_5A_T9: Test list requires admin token**
- [ ] **MDB002_5A_T10: Test info returns namespace stats**

### Phase 6: SSE Subscriptions (Day 17-20)

#### Phase MDB002_6A: CODE: SSE Handler and Subscriptions
- [ ] Create `internal/api/sse.go` - SSE handler
- [ ] Implement SSE connection management
- [ ] Parse query parameters (stream/category, position, consumer)
- [ ] Validate token from query string or header
- [ ] Set SSE headers (Content-Type: text/event-stream)
- [ ] Handle connection lifecycle and cleanup
- [ ] Implement stream subscription logic
- [ ] Poll database for new messages
- [ ] Send poke events (stream, position, globalPosition)
- [ ] Implement category subscription logic
- [ ] Apply consumer group filtering
- [ ] Send poke events with stream name
- [ ] Implement backoff/throttling

#### Phase MDB002_6A: TESTS: SSE Subscription Tests
- [ ] **MDB002_6A_T1: Test SSE connection established**
- [ ] **MDB002_6A_T2: Test SSE headers set correctly**
- [ ] **MDB002_6A_T3: Test connection requires valid token**
- [ ] **MDB002_6A_T4: Test stream subscription receives poke on new message**
- [ ] **MDB002_6A_T5: Test poke contains correct position**
- [ ] **MDB002_6A_T6: Test multiple pokes for multiple messages**
- [ ] **MDB002_6A_T7: Test subscription from specific position**
- [ ] **MDB002_6A_T8: Test category subscription receives pokes**
- [ ] **MDB002_6A_T9: Test poke includes stream name for category**
- [ ] **MDB002_6A_T10: Test consumer group filtering in subscription**
- [ ] **MDB002_6A_T11: Test connection cleanup on client disconnect**

### Phase 7: Test Mode & Integration (Day 21-23)

#### Phase MDB002_7A: CODE: Test Mode and System Operations
- [ ] Add --test-mode flag to server
- [ ] Enable SQLite in-memory backend in test mode
- [ ] Implement auto-namespace creation on first write
- [ ] Return token in X-MessageDB-Token header
- [ ] Disable auth requirement in test mode
- [ ] Add test mode indicator to logs
- [ ] Implement `handleSystemVersion()` in handlers.go
- [ ] Return server version string
- [ ] Implement `handleSystemHealth()` in handlers.go
- [ ] Check database connection
- [ ] Return backend type and stats
- [ ] Add comprehensive logging
- [ ] Implement request/response timing
- [ ] Add connection pooling configuration
- [ ] Optimize database queries
- [ ] Add performance benchmarks

#### Phase MDB002_7A: TESTS: Test Mode and Integration Tests
- [ ] **MDB002_7A_T1: Test mode uses in-memory SQLite**
- [ ] **MDB002_7A_T2: Test auto-namespace creation on first write**
- [ ] **MDB002_7A_T3: Test token returned in response header**
- [ ] **MDB002_7A_T4: Test auth not required in test mode**
- [ ] **MDB002_7A_T5: Test sys.version returns version**
- [ ] **MDB002_7A_T6: Test sys.health returns status**
- [ ] **MDB002_7A_T7: Test complete workflow: create ns → write → read**
- [ ] **MDB002_7A_T8: Test namespace isolation end-to-end**
- [ ] **MDB002_7A_T9: Test subscription + write + fetch workflow**
- [ ] **MDB002_7A_T10: Test optimistic locking workflow**
- [ ] **MDB002_7A_T11: Test performance: API response < 50ms (p95)**
- [ ] **MDB002_7A_T12: Test concurrent writes to different namespaces**

## Development Workflow Per Phase

For **EACH** phase:

1. **Implement Code** (Phase XA CODE)
2. **Write Tests IMMEDIATELY** (Phase XA TESTS)
3. **Run Tests & Verify** - All tests must pass (`go test ./...`)
4. **Run Linting** - `golangci-lint run` or `go vet ./...`
5. **Commit with good message** - Only if tests pass
6. **NEVER move to next phase with failing tests**

## File Structure

```
message-db/
├── cmd/
│   └── messagedb/
│       └── main.go                    # Server entry point, startup logic
├── internal/
│   ├── api/
│   │   ├── rpc.go                     # RPC handler and routing
│   │   ├── sse.go                     # SSE subscriptions
│   │   ├── handlers.go                # RPC method implementations
│   │   └── middleware.go              # Auth middleware
│   ├── auth/
│   │   └── token.go                   # Token generation/validation
│   └── store/
│       ├── store.go                   # Store interface
│       ├── postgres/
│       │   └── postgres.go            # Postgres implementation
│       └── sqlite/
│           └── sqlite.go              # SQLite implementation
└── test/
    ├── integration/
    │   ├── stream_test.go             # Stream operation tests
    │   ├── category_test.go           # Category operation tests
    │   ├── namespace_test.go          # Namespace operation tests
    │   ├── subscription_test.go       # SSE subscription tests
    │   └── testmode_test.go           # Test mode tests
    └── unit/
        ├── rpc_test.go                # RPC parsing tests
        ├── token_test.go              # Token tests
        └── middleware_test.go         # Auth middleware tests
```

## Code Size Estimates

```
cmd/messagedb/main.go:          ~150 lines  (server setup, default namespace)
internal/api/rpc.go:            ~200 lines  (RPC parsing, routing, errors)
internal/api/handlers.go:       ~600 lines  (all RPC methods)
internal/api/sse.go:            ~250 lines  (SSE connection management)
internal/api/middleware.go:     ~100 lines  (auth middleware)
internal/auth/token.go:         ~80 lines   (token functions)

Total implementation:           ~1380 lines
Tests:                          ~2000 lines (60+ test scenarios)
```

## Key Implementation Details

**Token Format:**
```go
func GenerateToken(namespace string) (string, error) {
    randomBytes := make([]byte, 32)
    if _, err := rand.Read(randomBytes); err != nil {
        return "", err
    }
    
    nsEncoded := base64.RawURLEncoding.EncodeToString([]byte(namespace))
    randomHex := hex.EncodeToString(randomBytes)
    
    return fmt.Sprintf("ns_%s_%s", nsEncoded, randomHex), nil
}
```

**RPC Routing:**
```go
func (h *RPCHandler) Handle(w http.ResponseWriter, r *http.Request) {
    var req []interface{}
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, "INVALID_REQUEST", "Malformed JSON", nil)
        return
    }
    
    if len(req) < 1 {
        writeError(w, "INVALID_REQUEST", "Missing method", nil)
        return
    }
    
    method, ok := req[0].(string)
    if !ok {
        writeError(w, "INVALID_REQUEST", "Method must be string", nil)
        return
    }
    
    args := req[1:]
    result, err := h.route(r.Context(), method, args)
    if err != nil {
        writeRPCError(w, err)
        return
    }
    
    writeSuccess(w, result)
}
```

**SSE Poke:**
```go
func (s *SSEHandler) sendPoke(w http.ResponseWriter, poke Poke) error {
    data, _ := json.Marshal(poke)
    _, err := fmt.Fprintf(w, "event: poke\ndata: %s\n\n", data)
    if err != nil {
        return err
    }
    w.(http.Flusher).Flush()
    return nil
}
```

**Auth Middleware:**
```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            writeError(w, "AUTH_REQUIRED", "Authorization header required", nil)
            return
        }
        
        token := strings.TrimPrefix(authHeader, "Bearer ")
        namespace, err := auth.ParseToken(token)
        if err != nil {
            writeError(w, "AUTH_INVALID_TOKEN", "Invalid token format", nil)
            return
        }
        
        // Validate token in database
        valid, err := validateToken(token, namespace)
        if !valid || err != nil {
            writeError(w, "AUTH_UNAUTHORIZED", "Token not authorized", nil)
            return
        }
        
        // Add namespace to context
        ctx := context.WithValue(r.Context(), "namespace", namespace)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## Test Distribution Summary

- **Phase 1 Tests:** 10 scenarios (Server setup + RPC parsing)
- **Phase 2 Tests:** 11 scenarios (Auth + tokens + default namespace)
- **Phase 3 Tests:** 12 scenarios (Stream operations)
- **Phase 4 Tests:** 8 scenarios (Category operations)
- **Phase 5 Tests:** 10 scenarios (Namespace management)
- **Phase 6 Tests:** 11 scenarios (SSE subscriptions)
- **Phase 7 Tests:** 12 scenarios (Test mode + integration)

**Total: 74 test scenarios covering all Epic MDB002 acceptance criteria**

## Dependencies

- **MDB001:** Core storage interface and migrations complete
- **Go 1.21+:** HTTP server, crypto/rand, encoding/json
- **Chi router (optional):** For HTTP routing (or use stdlib)
- **Database drivers:** github.com/lib/pq (Postgres), github.com/mattn/go-sqlite3

## Performance Targets

| Operation | Target |
|-----------|--------|
| RPC routing | <1ms |
| Token validation | <1ms |
| stream.write | <10ms (p95) |
| stream.get | <20ms (p95) |
| category.get | <30ms (p95) |
| SSE poke | <5ms |
| Namespace creation | <100ms |

---

## Implementation Status

### EPIC MDB002: RPC API & AUTHENTICATION - PENDING
### Current Status: READY FOR IMPLEMENTATION

### Progress Tracking
- [ ] Phase MDB002_1A: HTTP Server & RPC Foundation
- [ ] Phase MDB002_2A: Authentication & Default Namespace
- [ ] Phase MDB002_3A: Stream Operations
- [ ] Phase MDB002_4A: Category Operations
- [ ] Phase MDB002_5A: Namespace Management
- [ ] Phase MDB002_6A: SSE Subscriptions
- [ ] Phase MDB002_7A: Test Mode & Integration

### Definition of Done
- [ ] HTTP server with /rpc endpoint
- [ ] All stream operations (write, get, last, version)
- [ ] All category operations (get with consumer groups)
- [ ] All namespace operations (create, delete, list, info)
- [ ] All system operations (version, health)
- [ ] Token generation and validation
- [ ] Auth middleware with namespace isolation
- [ ] SSE subscriptions (stream + category)
- [ ] Test mode with auto-namespace creation
- [ ] Default namespace on startup
- [ ] All 74 test scenarios passing
- [ ] Performance targets met
- [ ] Linting passes
- [ ] Documentation complete

### Important Rules
- ✅ Code compiles and tests pass before next phase
- ✅ Epic ID + test ID in test names (MDB002_XA_TN)
- ✅ Token format: ns_<base64(ns)>_<random_hex>
- ✅ Header auth: Authorization: Bearer <token>
- ✅ SSE pokes only (no full message data)
- ✅ Namespace isolation enforced
- ✅ Test mode uses SQLite in-memory
- ✅ Integration tests in test/integration/
