# ISSUE007: Golang SDK for EventoDB

**Status**: Not Started  
**Priority**: High  
**Effort**: 4-6 hours  
**Created**: 2024-12-20  

---

## **Overview**

Implement a minimal, idiomatic Golang SDK for EventoDB that passes all tests defined in `docs/SDK-TEST-SPEC.md`. The SDK will use standard library HTTP client and follow Go conventions.

**Location**: `clients/eventodb-go/`

**Key Principles**:
- Minimal dependencies (stdlib only, no external HTTP libraries)
- Idiomatic Go code (interfaces, error handling, contexts)
- Use standard `net/http` and `encoding/json`
- Tests run against live backend server
- Each test creates its own namespace for isolation

---

## **Implementation Plan**

### Phase 1: Project Setup (30 min)

**1.1 Initialize Go Module**
```bash
cd clients
mkdir eventodb-go
cd eventodb-go
go mod init github.com/eventodb/eventodb-go
```

**1.2 Project Structure**
```
clients/eventodb-go/
├── client.go              # Main client and API methods
├── types.go               # Type definitions
├── errors.go              # Error types
├── client_test.go         # Tests (using testing package)
├── write_test.go          # WRITE-* tests
├── read_test.go           # READ-* tests
├── last_test.go           # LAST-* tests
├── version_test.go        # VERSION-* tests
├── category_test.go       # CATEGORY-* tests
├── namespace_test.go      # NS-* tests
├── system_test.go         # SYS-* tests
├── auth_test.go           # AUTH-* tests
├── error_test.go          # ERROR-* tests
├── encoding_test.go       # ENCODING-* tests
├── testhelpers_test.go    # Test utilities
├── go.mod
├── go.sum
├── README.md
└── run_tests.sh           # Test runner script
```

**1.3 Dependencies**
```bash
# No external dependencies needed - stdlib only!
```

---

### Phase 2: Core Implementation (2 hours)

**2.1 Types Module** (`types.go`)

```go
package eventodb

import "time"

// Message represents a message to be written to a stream
type Message struct {
    Type     string                 `json:"type"`
    Data     map[string]interface{} `json:"data"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// WriteOptions configures message write operations
type WriteOptions struct {
    ID              *string `json:"id,omitempty"`
    ExpectedVersion *int64  `json:"expectedVersion,omitempty"`
}

// WriteResult contains the result of a write operation
type WriteResult struct {
    Position       int64 `json:"position"`
    GlobalPosition int64 `json:"globalPosition"`
}

// StreamMessage represents a message from stream.get
// Format: [id, type, position, globalPosition, data, metadata, time]
type StreamMessage struct {
    ID             string
    Type           string
    Position       int64
    GlobalPosition int64
    Data           map[string]interface{}
    Metadata       map[string]interface{}
    Time           time.Time
}

// CategoryMessage represents a message from category.get
// Format: [id, streamName, type, position, globalPosition, data, metadata, time]
type CategoryMessage struct {
    ID             string
    StreamName     string
    Type           string
    Position       int64
    GlobalPosition int64
    Data           map[string]interface{}
    Metadata       map[string]interface{}
    Time           time.Time
}

// GetStreamOptions configures stream read operations
type GetStreamOptions struct {
    Position       *int64 `json:"position,omitempty"`
    GlobalPosition *int64 `json:"globalPosition,omitempty"`
    BatchSize      *int   `json:"batchSize,omitempty"`
}

// GetCategoryOptions configures category read operations
type GetCategoryOptions struct {
    Position      *int64          `json:"position,omitempty"`
    BatchSize     *int            `json:"batchSize,omitempty"`
    Correlation   *string         `json:"correlation,omitempty"`
    ConsumerGroup *ConsumerGroup  `json:"consumerGroup,omitempty"`
}

// ConsumerGroup for distributed consumption
type ConsumerGroup struct {
    Member int `json:"member"`
    Size   int `json:"size"`
}

// GetLastOptions configures stream.last operations
type GetLastOptions struct {
    Type *string `json:"type,omitempty"`
}

// CreateNamespaceOptions configures namespace creation
type CreateNamespaceOptions struct {
    Description *string `json:"description,omitempty"`
    Token       *string `json:"token,omitempty"`
}

// NamespaceResult contains result of namespace creation
type NamespaceResult struct {
    Namespace string    `json:"namespace"`
    Token     string    `json:"token"`
    CreatedAt time.Time `json:"createdAt"`
}

// DeleteNamespaceResult contains result of namespace deletion
type DeleteNamespaceResult struct {
    Namespace       string    `json:"namespace"`
    DeletedAt       time.Time `json:"deletedAt"`
    MessagesDeleted int64     `json:"messagesDeleted"`
}

// NamespaceInfo contains namespace metadata
type NamespaceInfo struct {
    Namespace    string    `json:"namespace"`
    Description  string    `json:"description"`
    CreatedAt    time.Time `json:"createdAt"`
    MessageCount int64     `json:"messageCount"`
}

// HealthStatus contains server health info
type HealthStatus struct {
    Status string `json:"status"`
}
```

**2.2 Errors Module** (`errors.go`)

```go
package eventodb

import "fmt"

// Error represents a EventoDB error
type Error struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
    if len(e.Details) > 0 {
        return fmt.Sprintf("%s: %s (details: %v)", e.Code, e.Message, e.Details)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is allows error comparison
func (e *Error) Is(target error) bool {
    t, ok := target.(*Error)
    if !ok {
        return false
    }
    return e.Code == t.Code
}

// Common error codes
var (
    ErrAuthRequired        = &Error{Code: "AUTH_REQUIRED", Message: "authentication required"}
    ErrAuthInvalid         = &Error{Code: "AUTH_INVALID", Message: "invalid authentication"}
    ErrNamespaceExists     = &Error{Code: "NAMESPACE_EXISTS", Message: "namespace already exists"}
    ErrNamespaceNotFound   = &Error{Code: "NAMESPACE_NOT_FOUND", Message: "namespace not found"}
    ErrVersionConflict     = &Error{Code: "STREAM_VERSION_CONFLICT", Message: "stream version conflict"}
    ErrInvalidRequest      = &Error{Code: "INVALID_REQUEST", Message: "invalid request"}
)
```

**2.3 Client Module** (`client.go`)

```go
package eventodb

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Client is a EventoDB client
type Client struct {
    baseURL    string
    token      string
    httpClient *http.Client
}

// NewClient creates a new EventoDB client
func NewClient(baseURL string, opts ...ClientOption) *Client {
    c := &Client{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
    
    for _, opt := range opts {
        opt(c)
    }
    
    return c
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithToken sets the authentication token
func WithToken(token string) ClientOption {
    return func(c *Client) {
        c.token = token
    }
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
    return func(c *Client) {
        c.httpClient = httpClient
    }
}

// rpc makes an RPC call to the server
func (c *Client) rpc(ctx context.Context, method string, args ...interface{}) (json.RawMessage, error) {
    // Build RPC request: [method, ...args]
    reqBody := []interface{}{method}
    reqBody = append(reqBody, args...)
    
    body, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }
    
    req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rpc", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    if c.token != "" {
        req.Header.Set("Authorization", "Bearer "+c.token)
    }
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Capture token from response (for test mode auto-creation)
    if newToken := resp.Header.Get("X-EventoDB-Token"); newToken != "" && c.token == "" {
        c.token = newToken
    }
    
    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }
    
    // Handle error responses
    if resp.StatusCode != http.StatusOK {
        var errResp struct {
            Error *Error `json:"error"`
        }
        if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
            return nil, errResp.Error
        }
        return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
    }
    
    return respBody, nil
}

// GetToken returns the current authentication token
func (c *Client) GetToken() string {
    return c.token
}

// Stream Operations

func (c *Client) StreamWrite(ctx context.Context, streamName string, message Message, opts *WriteOptions) (*WriteResult, error) {
    if opts == nil {
        opts = &WriteOptions{}
    }
    
    result, err := c.rpc(ctx, "stream.write", streamName, message, opts)
    if err != nil {
        return nil, err
    }
    
    var wr WriteResult
    if err := json.Unmarshal(result, &wr); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    return &wr, nil
}

func (c *Client) StreamGet(ctx context.Context, streamName string, opts *GetStreamOptions) ([]StreamMessage, error) {
    if opts == nil {
        opts = &GetStreamOptions{}
    }
    
    result, err := c.rpc(ctx, "stream.get", streamName, opts)
    if err != nil {
        return nil, err
    }
    
    // Parse array of arrays into StreamMessage structs
    var raw [][]interface{}
    if err := json.Unmarshal(result, &raw); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    messages := make([]StreamMessage, len(raw))
    for i, msg := range raw {
        if err := parseStreamMessage(&messages[i], msg); err != nil {
            return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
        }
    }
    
    return messages, nil
}

func (c *Client) StreamLast(ctx context.Context, streamName string, opts *GetLastOptions) (*StreamMessage, error) {
    if opts == nil {
        opts = &GetLastOptions{}
    }
    
    result, err := c.rpc(ctx, "stream.last", streamName, opts)
    if err != nil {
        return nil, err
    }
    
    // Check for null
    if string(result) == "null" {
        return nil, nil
    }
    
    var raw []interface{}
    if err := json.Unmarshal(result, &raw); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    var msg StreamMessage
    if err := parseStreamMessage(&msg, raw); err != nil {
        return nil, err
    }
    
    return &msg, nil
}

func (c *Client) StreamVersion(ctx context.Context, streamName string) (*int64, error) {
    result, err := c.rpc(ctx, "stream.version", streamName)
    if err != nil {
        return nil, err
    }
    
    // Check for null
    if string(result) == "null" {
        return nil, nil
    }
    
    var version int64
    if err := json.Unmarshal(result, &version); err != nil {
        return nil, fmt.Errorf("failed to unmarshal version: %w", err)
    }
    
    return &version, nil
}

// Category Operations

func (c *Client) CategoryGet(ctx context.Context, categoryName string, opts *GetCategoryOptions) ([]CategoryMessage, error) {
    if opts == nil {
        opts = &GetCategoryOptions{}
    }
    
    result, err := c.rpc(ctx, "category.get", categoryName, opts)
    if err != nil {
        return nil, err
    }
    
    var raw [][]interface{}
    if err := json.Unmarshal(result, &raw); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    messages := make([]CategoryMessage, len(raw))
    for i, msg := range raw {
        if err := parseCategoryMessage(&messages[i], msg); err != nil {
            return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
        }
    }
    
    return messages, nil
}

// Namespace Operations

func (c *Client) NamespaceCreate(ctx context.Context, namespaceID string, opts *CreateNamespaceOptions) (*NamespaceResult, error) {
    if opts == nil {
        opts = &CreateNamespaceOptions{}
    }
    
    result, err := c.rpc(ctx, "ns.create", namespaceID, opts)
    if err != nil {
        return nil, err
    }
    
    var nr NamespaceResult
    if err := json.Unmarshal(result, &nr); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    // Update client token if received
    if nr.Token != "" && c.token == "" {
        c.token = nr.Token
    }
    
    return &nr, nil
}

func (c *Client) NamespaceDelete(ctx context.Context, namespaceID string) (*DeleteNamespaceResult, error) {
    result, err := c.rpc(ctx, "ns.delete", namespaceID)
    if err != nil {
        return nil, err
    }
    
    var dr DeleteNamespaceResult
    if err := json.Unmarshal(result, &dr); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    return &dr, nil
}

func (c *Client) NamespaceList(ctx context.Context) ([]NamespaceInfo, error) {
    result, err := c.rpc(ctx, "ns.list")
    if err != nil {
        return nil, err
    }
    
    var namespaces []NamespaceInfo
    if err := json.Unmarshal(result, &namespaces); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    return namespaces, nil
}

func (c *Client) NamespaceInfo(ctx context.Context, namespaceID string) (*NamespaceInfo, error) {
    result, err := c.rpc(ctx, "ns.info", namespaceID)
    if err != nil {
        return nil, err
    }
    
    var info NamespaceInfo
    if err := json.Unmarshal(result, &info); err != nil {
        return nil, fmt.Errorf("failed to unmarshal result: %w", err)
    }
    
    return &info, nil
}

// System Operations

func (c *Client) SystemVersion(ctx context.Context) (string, error) {
    result, err := c.rpc(ctx, "sys.version")
    if err != nil {
        return "", err
    }
    
    var version string
    if err := json.Unmarshal(result, &version); err != nil {
        return "", fmt.Errorf("failed to unmarshal version: %w", err)
    }
    
    return version, nil
}

func (c *Client) SystemHealth(ctx context.Context) (*HealthStatus, error) {
    result, err := c.rpc(ctx, "sys.health")
    if err != nil {
        return nil, err
    }
    
    var health HealthStatus
    if err := json.Unmarshal(result, &health); err != nil {
        return nil, fmt.Errorf("failed to unmarshal health: %w", err)
    }
    
    return &health, nil
}

// Helper functions for parsing message arrays

func parseStreamMessage(msg *StreamMessage, raw []interface{}) error {
    if len(raw) != 7 {
        return fmt.Errorf("expected 7 fields, got %d", len(raw))
    }
    
    msg.ID = raw[0].(string)
    msg.Type = raw[1].(string)
    msg.Position = int64(raw[2].(float64))
    msg.GlobalPosition = int64(raw[3].(float64))
    msg.Data = raw[4].(map[string]interface{})
    
    if raw[5] != nil {
        msg.Metadata = raw[5].(map[string]interface{})
    }
    
    timeStr := raw[6].(string)
    t, err := time.Parse(time.RFC3339Nano, timeStr)
    if err != nil {
        return fmt.Errorf("failed to parse time: %w", err)
    }
    msg.Time = t
    
    return nil
}

func parseCategoryMessage(msg *CategoryMessage, raw []interface{}) error {
    if len(raw) != 8 {
        return fmt.Errorf("expected 8 fields, got %d", len(raw))
    }
    
    msg.ID = raw[0].(string)
    msg.StreamName = raw[1].(string)
    msg.Type = raw[2].(string)
    msg.Position = int64(raw[3].(float64))
    msg.GlobalPosition = int64(raw[4].(float64))
    msg.Data = raw[5].(map[string]interface{})
    
    if raw[6] != nil {
        msg.Metadata = raw[6].(map[string]interface{})
    }
    
    timeStr := raw[7].(string)
    t, err := time.Parse(time.RFC3339Nano, timeStr)
    if err != nil {
        return fmt.Errorf("failed to parse time: %w", err)
    }
    msg.Time = t
    
    return nil
}
```

---

### Phase 3: Test Infrastructure (1 hour)

**3.1 Test Helpers** (`testhelpers_test.go`)

```go
package eventodb

import (
    "context"
    "fmt"
    "os"
    "testing"
    "time"
)

var (
    testBaseURL = getEnv("MESSAGEDB_URL", "http://localhost:8080")
    testAdminToken = getEnv("MESSAGEDB_ADMIN_TOKEN", "")
)

type testContext struct {
    client      *Client
    namespaceID string
    t           *testing.T
}

func setupTest(t *testing.T, testName string) *testContext {
    t.Helper()
    
    // Create admin client
    adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
    
    // Create test namespace
    namespaceID := fmt.Sprintf("test-%s-%d", testName, time.Now().UnixNano())
    
    ctx := context.Background()
    result, err := adminClient.NamespaceCreate(ctx, namespaceID, &CreateNamespaceOptions{
        Description: strPtr(fmt.Sprintf("Test namespace for %s", testName)),
    })
    if err != nil {
        t.Fatalf("Failed to create test namespace: %v", err)
    }
    
    // Create client with namespace token
    client := NewClient(testBaseURL, WithToken(result.Token))
    
    tc := &testContext{
        client:      client,
        namespaceID: namespaceID,
        t:           t,
    }
    
    // Register cleanup
    t.Cleanup(func() {
        tc.cleanup()
    })
    
    return tc
}

func (tc *testContext) cleanup() {
    // Delete test namespace
    adminClient := NewClient(testBaseURL, WithToken(testAdminToken))
    ctx := context.Background()
    _, _ = adminClient.NamespaceDelete(ctx, tc.namespaceID)
}

func randomStreamName() string {
    return fmt.Sprintf("test-%d", time.Now().UnixNano())
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func strPtr(s string) *string {
    return &s
}

func int64Ptr(i int64) *int64 {
    return &i
}

func intPtr(i int) *int {
    return &i
}
```

**3.2 Test Structure Pattern**

```go
package eventodb

import (
    "context"
    "testing"
)

func TestWRITE001_WriteMinimalMessage(t *testing.T) {
    tc := setupTest(t, "write-001")
    ctx := context.Background()
    
    stream := randomStreamName()
    result, err := tc.client.StreamWrite(ctx, stream, Message{
        Type: "TestEvent",
        Data: map[string]interface{}{"foo": "bar"},
    }, nil)
    
    if err != nil {
        t.Fatalf("Failed to write message: %v", err)
    }
    
    if result.Position < 0 {
        t.Errorf("Expected position >= 0, got %d", result.Position)
    }
    
    if result.GlobalPosition < 0 {
        t.Errorf("Expected globalPosition >= 0, got %d", result.GlobalPosition)
    }
    
    if result.Position != 0 {
        t.Errorf("Expected first message at position 0, got %d", result.Position)
    }
}
```

---

### Phase 4: Test Implementation (2 hours)

Implement tests in priority order following `docs/SDK-TEST-SPEC.md`:

**Tier 1 (Must Have) - 1.5 hours**
- `write_test.go`: WRITE-001 through WRITE-009
- `read_test.go`: READ-001 through READ-010
- `auth_test.go`: AUTH-001 through AUTH-004
- `error_test.go`: ERROR-001 through ERROR-004

**Tier 2 (Should Have) - 30 min**
- `last_test.go`: LAST-001 through LAST-004
- `version_test.go`: VERSION-001 through VERSION-003
- `category_test.go`: CATEGORY-001 through CATEGORY-008
- `namespace_test.go`: NS-001 through NS-008
- `system_test.go`: SYS-001, SYS-002

**Tier 3 (Nice to Have) - Future**
- `encoding_test.go`: ENCODING-001 through ENCODING-010
- Edge case tests
- SSE tests (requires additional implementation)

---

### Phase 5: Documentation & Polish (30 min)

**5.1 README.md**
```markdown
# eventodb-go

Go client for EventoDB - a simple, fast message store.

## Installation

```bash
go get github.com/eventodb/eventodb-go
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    eventodb "github.com/eventodb/eventodb-go"
)

func main() {
    // Create client
    client := eventodb.NewClient("http://localhost:8080", 
        eventodb.WithToken("ns_..."))
    
    ctx := context.Background()
    
    // Write message
    result, err := client.StreamWrite(ctx, "account-123", eventodb.Message{
        Type: "Deposited",
        Data: map[string]interface{}{"amount": 100},
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Written at position %d\n", result.Position)
    
    // Read stream
    messages, err := client.StreamGet(ctx, "account-123", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Read %d messages\n", len(messages))
}
```

## Testing

Tests run against a live EventoDB server:

```bash
# Start server
docker-compose up -d

# Run tests
go test -v

# With custom URL
MESSAGEDB_URL=http://localhost:8080 go test -v
```

## API Reference

See [GoDoc](https://pkg.go.dev/github.com/eventodb/eventodb-go) for full API documentation.
```

**5.2 Test Runner Script** (`run_tests.sh`)
```bash
#!/bin/bash
set -e

echo "Running EventoDB Go SDK tests..."
echo "Server: ${MESSAGEDB_URL:-http://localhost:8080}"
echo ""

go test -v -race -timeout 30s ./...
```

**5.3 Go Documentation**
- Add package-level documentation
- Document all public types and methods
- Follow Go doc conventions

---

## **Success Criteria**

- [ ] All Tier 1 tests passing (WRITE, READ, AUTH, ERROR)
- [ ] All Tier 2 tests passing (CATEGORY, NS, SYS, LAST, VERSION)
- [ ] Clean, idiomatic Go code
- [ ] Zero external dependencies (stdlib only)
- [ ] Tests create/cleanup their own namespaces
- [ ] README with clear usage examples
- [ ] Full GoDoc documentation
- [ ] `go vet` passes with no issues
- [ ] `go test -race` passes (race detector)

---

## **API Design Notes**

### Context-First
All operations accept `context.Context` as first parameter (Go convention):
```go
result, err := client.StreamWrite(ctx, streamName, message, nil)
```

### Pointer Options
Options are pointers to allow `nil` for defaults:
```go
// Default options
messages, err := client.StreamGet(ctx, stream, nil)

// With options
messages, err := client.StreamGet(ctx, stream, &GetStreamOptions{
    Position: int64Ptr(5),
    BatchSize: intPtr(10),
})
```

### Error Handling
```go
result, err := client.StreamWrite(ctx, stream, message, nil)
if err != nil {
    var dbErr *eventodb.Error
    if errors.As(err, &dbErr) {
        if errors.Is(dbErr, eventodb.ErrVersionConflict) {
            // Handle version conflict
        }
    }
    return err
}
```

### Typed Messages
```go
// StreamMessage from stream.get
msg := messages[0]
fmt.Printf("ID: %s, Type: %s, Position: %d\n", msg.ID, msg.Type, msg.Position)

// CategoryMessage from category.get
catMsg := catMessages[0]
fmt.Printf("Stream: %s, Type: %s\n", catMsg.StreamName, catMsg.Type)
```

---

## **Implementation Phases**

### Phase 1: Foundation (30 min) ✓ DOCUMENTED
- [x] Project structure
- [x] Type definitions
- [x] Error types
- [x] Test helpers

### Phase 2: Core Client (2 hours) ✓ DOCUMENTED
- [x] HTTP/RPC client
- [x] Token management
- [x] Stream operations
- [x] Category operations
- [x] Namespace operations
- [x] System operations

### Phase 3: Tier 1 Tests (1.5 hours)
- [ ] WRITE tests (WRITE-001 to WRITE-009)
- [ ] READ tests (READ-001 to READ-010)
- [ ] AUTH tests (AUTH-001 to AUTH-004)
- [ ] ERROR tests (ERROR-001 to ERROR-004)

### Phase 4: Tier 2 Tests (30 min)
- [ ] LAST tests
- [ ] VERSION tests
- [ ] CATEGORY tests
- [ ] NAMESPACE tests
- [ ] SYSTEM tests

### Phase 5: Polish (30 min)
- [ ] README documentation
- [ ] GoDoc comments
- [ ] Test runner script
- [ ] go vet / go fmt

---

## **Go-Specific Considerations**

### Stdlib Only
- Use `net/http` for HTTP client (no external deps like resty)
- Use `encoding/json` for JSON (already performant)
- Use `context.Context` for cancellation/timeouts

### Type Safety
- Strongly typed message structures
- Use pointer types for optional fields
- Leverage Go's type system for compile-time safety

### Testing
- Use standard `testing` package
- Table-driven tests where appropriate
- Subtests with `t.Run()` for organization
- Race detector (`-race` flag)

### Idiomatic Patterns
- Errors as values (not exceptions)
- Constructor functions (`New...`)
- Functional options pattern (`WithToken()`)
- Interface-based design (future: mockable client)

---

## **Potential Enhancements** (Future)

1. **Context Timeout Helpers**: Helper functions for common timeout patterns
2. **Retry Logic**: Configurable retry with exponential backoff
3. **Connection Pooling**: Leverage http.Transport for connection reuse
4. **Structured Logging**: Integration with slog (Go 1.21+)
5. **Generic Type Support**: Type-safe message data with generics (Go 1.18+)
6. **SSE Client**: Server-Sent Events subscription support
7. **Middleware**: Request/response middleware hooks

---

## **References**

- Test Spec: `docs/SDK-TEST-SPEC.md`
- Elixir SDK: `clients/eventodb_ex/`
- Node.js SDK: `clients/eventodb-node/`
- Go Package Layout: https://github.com/golang-standards/project-layout
- Effective Go: https://go.dev/doc/effective_go
