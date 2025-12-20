# ISSUE005: Golang-Based SDK Spec Test Suite

**Status**: Proposed  
**Priority**: High  
**Created**: 2024-12-19  
**Related**: `docs/SDK-TEST-SPEC.md`

## Problem Statement

We need a comprehensive test suite based on `docs/SDK-TEST-SPEC.md` that can validate all EventoDB backends (SQLite, Postgres, Pebble) against the same specification to ensure consistent behavior.

## Decision: Golang Implementation ✅

Implement the SDK test suite in Golang rather than in `test_external/tests`.

## Rationale

### Advantages of Golang Approach

1. **Single test codebase** - Write once, run against all backends (SQLite, Postgres, Pebble)
2. **Backend switching via environment variables** - Easy CI/CD integration
3. **Performance** - Faster test execution compared to external HTTP/RPC tests
4. **Consistency** - Tests live with the server code, easier to maintain in sync
5. **Better error messages** - Direct access to server internals when debugging
6. **Existing infrastructure** - Already have `test_integration` setup with helper functions

### Disadvantages of test_external/tests

1. **Network overhead** - Every test makes HTTP/RPC calls
2. **Slower execution** - HTTP round-trips add latency
3. **Requires running server** - More complex test orchestration
4. **Language-specific** - TypeScript tests only validate the TS/Node.js SDK, not the server itself
5. **Less integration** - Can't easily test internal state or backend-specific behaviors

## Proposed Structure

```
golang/
├── test_integration/
│   ├── sdk_spec_test.go          # Main SDK spec test runner & helpers
│   ├── sdk_spec_write_test.go    # WRITE-001 to WRITE-010
│   ├── sdk_spec_read_test.go     # READ-001 to READ-010, LAST-001 to LAST-004
│   ├── sdk_spec_version_test.go  # VERSION-001 to VERSION-003
│   ├── sdk_spec_category_test.go # CATEGORY-001 to CATEGORY-008
│   ├── sdk_spec_namespace_test.go# NS-001 to NS-008
│   ├── sdk_spec_system_test.go   # SYS-001 to SYS-002
│   ├── sdk_spec_auth_test.go     # AUTH-001 to AUTH-004
│   ├── sdk_spec_error_test.go    # ERROR-001 to ERROR-007
│   ├── sdk_spec_encoding_test.go # ENCODING-001 to ENCODING-010
│   ├── sdk_spec_edge_test.go     # EDGE-001 to EDGE-008
│   └── sdk_spec_sse_test.go      # SSE-001 to SSE-008
```

## Key Implementation Details

### 1. Test Naming with IDs

```go
func TestWRITE001_WriteMinimalMessage(t *testing.T) {
    backends := getTestBackends() // sqlite, postgres, pebble
    for _, backend := range backends {
        t.Run(backend, func(t *testing.T) {
            ts := SetupTestServerWithBackend(t, backend)
            defer ts.Cleanup()
            
            // Test implementation matching SDK-TEST-SPEC.md WRITE-001
            stream := randomStreamName("test")
            msg := map[string]interface{}{
                "type": "TestEvent",
                "data": map[string]interface{}{"foo": "bar"},
            }
            
            result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
            require.NoError(t, err)
            
            // Expected: Returns object with position (>= 0) and globalPosition (>= 0)
            resultMap := result.(map[string]interface{})
            assert.GreaterOrEqual(t, resultMap["position"].(float64), 0.0)
            assert.GreaterOrEqual(t, resultMap["globalPosition"].(float64), 0.0)
            
            // First message should have position 0
            assert.Equal(t, 0.0, resultMap["position"].(float64))
        })
    }
}
```

### 2. Backend Configuration

```go
// In test_helpers.go or sdk_spec_test.go

func getTestBackends() []string {
    if backend := os.Getenv("TEST_BACKEND"); backend != "" {
        return []string{backend}
    }
    // Default: test all backends
    return []string{"sqlite", "postgres", "pebble"}
}

func SetupTestServerWithBackend(t *testing.T, backend string) *TestServer {
    // Create temporary directory for test data
    tmpDir := t.TempDir()
    
    var storageConfig string
    switch backend {
    case "sqlite":
        storageConfig = fmt.Sprintf("sqlite:%s/test.db", tmpDir)
    case "postgres":
        storageConfig = getTestPostgresURL() // from env or test container
    case "pebble":
        storageConfig = fmt.Sprintf("pebble:%s/pebble", tmpDir)
    default:
        t.Fatalf("Unknown backend: %s", backend)
    }
    
    // Start server with backend
    ts := &TestServer{
        Port:    getRandomPort(),
        TmpDir:  tmpDir,
        Backend: backend,
    }
    
    // ... server startup logic ...
    
    return ts
}
```

### 3. CI/CD Integration

```yaml
# .github/workflows/test.yml
name: SDK Spec Tests

jobs:
  test-sqlite:
    name: SDK Spec - SQLite
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run SDK spec tests
        run: TEST_BACKEND=sqlite go test -v ./golang/test_integration/sdk_spec_*
        
  test-postgres:
    name: SDK Spec - Postgres
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run SDK spec tests
        env:
          TEST_BACKEND: postgres
          TEST_POSTGRES_URL: postgres://postgres:postgres@localhost:5432/eventodb_test?sslmode=disable
        run: go test -v ./golang/test_integration/sdk_spec_*
        
  test-pebble:
    name: SDK Spec - Pebble
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run SDK spec tests
        run: TEST_BACKEND=pebble go test -v ./golang/test_integration/sdk_spec_*
```

### 4. Compliance Matrix Generation

```go
// sdk_spec_test.go

// TestGenerateComplianceMatrix runs after all tests and generates a markdown matrix
func TestGenerateComplianceMatrix(t *testing.T) {
    if os.Getenv("GENERATE_COMPLIANCE_MATRIX") != "true" {
        t.Skip("Set GENERATE_COMPLIANCE_MATRIX=true to generate compliance matrix")
    }
    
    // Parse test results and generate matrix
    matrix := `| Test ID | SQLite | Postgres | Pebble | Notes |
|---------|---------|----------|--------|-------|
| WRITE-001 | ✅ | ✅ | ✅ | |
| WRITE-002 | ✅ | ✅ | ✅ | |
...
`
    
    err := os.WriteFile("SDK-COMPLIANCE.md", []byte(matrix), 0644)
    require.NoError(t, err)
}
```

## When to Use test_external/tests

Keep `test_external/tests` for:

- **End-to-end testing** of the deployed system
- **SDK validation** for non-Go SDKs (Node.js, Elixir, etc.)
- **Regression testing** against production-like environments
- **Cross-language compatibility** verification

The external tests validate the **HTTP/RPC API contract**, while the Golang tests validate **backend correctness**.

## Implementation Plan

### Phase 1: Foundation (Priority: High)
- [ ] Create `sdk_spec_test.go` with helper functions
- [ ] Add `getTestBackends()` and `SetupTestServerWithBackend()`
- [ ] Implement WRITE-001 to WRITE-005 as examples
- [ ] Implement READ-001 to READ-003 as examples
- [ ] Verify all 3 backends pass the example tests

### Phase 2: Core Operations (Priority: High)
- [ ] Complete all WRITE tests (WRITE-001 to WRITE-010)
- [ ] Complete all READ tests (READ-001 to READ-010)
- [ ] Complete all LAST tests (LAST-001 to LAST-004)
- [ ] Complete all VERSION tests (VERSION-001 to VERSION-003)

### Phase 3: Category & Namespace (Priority: Medium)
- [ ] Complete all CATEGORY tests (CATEGORY-001 to CATEGORY-008)
- [ ] Complete all NS tests (NS-001 to NS-008)
- [ ] Complete all SYS tests (SYS-001 to SYS-002)

### Phase 4: Error Handling & Edge Cases (Priority: Medium)
- [ ] Complete all AUTH tests (AUTH-001 to AUTH-004)
- [ ] Complete all ERROR tests (ERROR-001 to ERROR-007)
- [ ] Complete all ENCODING tests (ENCODING-001 to ENCODING-010)
- [ ] Complete all EDGE tests (EDGE-001 to EDGE-008)

### Phase 5: SSE & Compliance (Priority: Low)
- [ ] Complete all SSE tests (SSE-001 to SSE-008)
- [ ] Implement compliance matrix generation
- [ ] Add CI/CD workflows for all backends
- [ ] Document backend-specific behaviors

## Success Criteria

- [ ] All 80+ tests from SDK-TEST-SPEC.md implemented in Golang
- [ ] Test names match spec IDs (e.g., `TestWRITE001_WriteMinimalMessage`)
- [ ] All tests pass on SQLite, Postgres, and Pebble backends
- [ ] Can run single backend via `TEST_BACKEND=sqlite go test ...`
- [ ] Can run all backends via `go test ...`
- [ ] CI/CD runs tests against all backends
- [ ] Compliance matrix auto-generated and tracked

## Open Questions

1. Should we run all backends in parallel or sequentially?
2. Do we need a separate compliance tracking file or embed in test output?
3. Should we add performance benchmarks alongside spec tests?
4. How to handle backend-specific edge cases (e.g., Postgres vs SQLite limits)?

## References

- `docs/SDK-TEST-SPEC.md` - Complete test specification
- `golang/test_integration/` - Existing integration test infrastructure
- `test_external/tests/` - External TypeScript tests (keep for SDK validation)
