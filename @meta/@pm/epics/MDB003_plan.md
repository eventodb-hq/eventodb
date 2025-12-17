# EPIC MDB003: Testing & Production Readiness Implementation Plan

## Test-Driven Development Approach

### Phase 1: External Test Client (Day 1-2)

#### Phase MDB003_1A: CODE: Bun.js Test Environment Setup
- [ ] Create `test/package.json` with Bun test dependencies
- [ ] Create `test/bun.lockb` lock file
- [ ] Set up test directory structure
- [ ] Create `test/lib/client.ts` - MessageDB TypeScript client
- [ ] Implement `MessageDBClient` class with RPC method
- [ ] Add `writeMessage()`, `getStream()`, `getCategory()` methods
- [ ] Add `subscribe()` method for SSE
- [ ] Add `deleteNamespace()` method
- [ ] Implement token management (capture from headers)
- [ ] Create `test/lib/helpers.ts` - test utilities
- [ ] Implement `startTestServer()` helper
- [ ] Implement `waitForHealthy()` port detection
- [ ] Add fixture loading utilities

#### Phase MDB003_1A: TESTS: Test Client Tests
- [ ] **MDB003_1A_T1: Test client can make RPC requests**
- [ ] **MDB003_1A_T2: Test client captures token from header**
- [ ] **MDB003_1A_T3: Test client sends token in Authorization header**
- [ ] **MDB003_1A_T4: Test client handles error responses**
- [ ] **MDB003_1A_T5: Test startTestServer spawns server**
- [ ] **MDB003_1A_T6: Test waitForHealthy detects port**
- [ ] **MDB003_1A_T7: Test server cleanup on test end**

### Phase 2: Stream & Category Tests (Day 3-4)

#### Phase MDB003_2A: CODE: Stream Operation Tests
- [ ] Create `test/tests/stream.test.ts`
- [ ] Implement basic write/read test
- [ ] Implement write with auto-generated ID test
- [ ] Implement write with provided ID test
- [ ] Implement optimistic locking success test
- [ ] Implement optimistic locking conflict test
- [ ] Implement stream.get with position filter test
- [ ] Implement stream.get with batchSize test
- [ ] Implement stream.last test
- [ ] Implement stream.version test
- [ ] Add namespace cleanup after each test

#### Phase MDB003_2A: TESTS: Stream Tests
- [ ] **MDB003_2A_T1: Test write and read message**
- [ ] **MDB003_2A_T2: Test write returns correct position**
- [ ] **MDB003_2A_T3: Test write with expectedVersion succeeds**
- [ ] **MDB003_2A_T4: Test optimistic locking prevents conflicts**
- [ ] **MDB003_2A_T5: Test get with position filter**
- [ ] **MDB003_2A_T6: Test get with batchSize limit**
- [ ] **MDB003_2A_T7: Test last message retrieval**
- [ ] **MDB003_2A_T8: Test stream version**
- [ ] **MDB003_2A_T9: Test empty stream returns empty array**
- [ ] **MDB003_2A_T10: Test message metadata preserved**

#### Phase MDB003_2B: CODE: Category Operation Tests
- [ ] Create `test/tests/category.test.ts`
- [ ] Implement basic category read test
- [ ] Implement consumer group partitioning test
- [ ] Implement consumer group no-overlap test
- [ ] Implement correlation filtering test
- [ ] Implement category with position filter test
- [ ] Implement category with batchSize test
- [ ] Add multiple stream writes for category tests

#### Phase MDB003_2B: TESTS: Category Tests
- [ ] **MDB003_2B_T1: Test category returns messages from multiple streams**
- [ ] **MDB003_2B_T2: Test category includes stream names**
- [ ] **MDB003_2B_T3: Test consumer groups partition streams**
- [ ] **MDB003_2B_T4: Test consumer groups have no overlap**
- [ ] **MDB003_2B_T5: Test correlation filtering**
- [ ] **MDB003_2B_T6: Test category with position filter**
- [ ] **MDB003_2B_T7: Test category with batchSize**
- [ ] **MDB003_2B_T8: Test empty category returns empty array**

### Phase 3: Subscription & Concurrency Tests (Day 5-6)

#### Phase MDB003_3A: CODE: SSE Subscription Tests
- [ ] Create `test/tests/subscription.test.ts`
- [ ] Implement stream subscription test
- [ ] Implement poke reception test
- [ ] Implement multiple pokes test
- [ ] Implement subscription from position test
- [ ] Implement category subscription test
- [ ] Implement consumer group in subscription test
- [ ] Add connection cleanup tests
- [ ] Handle SSE event parsing

#### Phase MDB003_3A: TESTS: Subscription Tests
- [ ] **MDB003_3A_T1: Test stream subscription receives pokes**
- [ ] **MDB003_3A_T2: Test poke contains correct position**
- [ ] **MDB003_3A_T3: Test multiple pokes for multiple messages**
- [ ] **MDB003_3A_T4: Test subscription from specific position**
- [ ] **MDB003_3A_T5: Test category subscription**
- [ ] **MDB003_3A_T6: Test poke includes stream name for category**
- [ ] **MDB003_3A_T7: Test connection cleanup on close**

#### Phase MDB003_3B: CODE: Concurrency & Isolation Tests
- [ ] Create `test/tests/concurrency.test.ts`
- [ ] Implement concurrent writes to different streams test
- [ ] Implement concurrent writes to same stream test
- [ ] Implement optimistic locking under concurrency test
- [ ] Create `test/tests/namespace.test.ts`
- [ ] Implement namespace isolation test
- [ ] Implement namespace auto-creation test
- [ ] Implement namespace deletion test

#### Phase MDB003_3B: TESTS: Concurrency & Isolation Tests
- [ ] **MDB003_3B_T1: Test concurrent writes to different streams**
- [ ] **MDB003_3B_T2: Test concurrent writes to same stream with locking**
- [ ] **MDB003_3B_T3: Test namespace isolation (no data leakage)**
- [ ] **MDB003_3B_T4: Test namespace auto-creation in test mode**
- [ ] **MDB003_3B_T5: Test namespace deletion**
- [ ] **MDB003_3B_T6: Test parallel test execution**

### Phase 4: Performance Benchmarks (Day 7)

#### Phase MDB003_4A: CODE: Benchmark Suite
- [ ] Create `test/benchmarks/` directory
- [ ] Create `test/benchmarks/performance.bench.ts`
- [ ] Implement stream write benchmark
- [ ] Implement stream read benchmark (100 messages)
- [ ] Implement category read benchmark
- [ ] Implement concurrent writes benchmark
- [ ] Create `test/benchmarks/baseline.json` for metrics
- [ ] Implement performance comparison script
- [ ] Add performance regression detection

#### Phase MDB003_4A: TESTS: Performance Validation
- [ ] **MDB003_4A_T1: Test stream write < 10ms (p95)**
- [ ] **MDB003_4A_T2: Test stream read (100 msgs) < 20ms (p95)**
- [ ] **MDB003_4A_T3: Test category read < 30ms (p95)**
- [ ] **MDB003_4A_T4: Test SSE poke delivery < 5ms**
- [ ] **MDB003_4A_T5: Test throughput > 1000 writes/sec**
- [ ] **MDB003_4A_T6: Test performance within 20% of baseline**

### Phase 5: Docker & Deployment (Day 8-9)

#### Phase MDB003_5A: CODE: Docker Setup
- [ ] Create `Dockerfile` with multi-stage build
- [ ] Create `.dockerignore` file
- [ ] Create `docker-compose.yml` with Postgres
- [ ] Add environment variable configuration
- [ ] Create `Makefile` with Docker commands
- [ ] Add health check to Dockerfile
- [ ] Create `deployments/kubernetes/` directory (optional)
- [ ] Add Kubernetes deployment manifest (optional)
- [ ] Add Kubernetes service manifest (optional)

#### Phase MDB003_5A: TESTS: Docker Validation
- [ ] **MDB003_5A_T1: Test Docker image builds successfully**
- [ ] **MDB003_5A_T2: Test Docker container starts**
- [ ] **MDB003_5A_T3: Test health endpoint accessible in container**
- [ ] **MDB003_5A_T4: Test docker-compose stack starts**
- [ ] **MDB003_5A_T5: Test Postgres connection in docker-compose**
- [ ] **MDB003_5A_T6: Test API accessible through Docker**

### Phase 6: CI/CD Pipeline (Day 10-11)

#### Phase MDB003_6A: CODE: GitHub Actions Workflows
- [ ] Create `.github/workflows/` directory
- [ ] Create `.github/workflows/test.yml` - main test workflow
- [ ] Add Go unit tests job
- [ ] Add Go linter job
- [ ] Add external Bun.js tests job
- [ ] Create `.github/workflows/benchmark.yml`
- [ ] Add performance benchmark job
- [ ] Add performance regression check
- [ ] Create `.github/workflows/docker.yml`
- [ ] Add Docker build job
- [ ] Add Docker push job (on release)
- [ ] Add test coverage reporting (optional)

#### Phase MDB003_6A: TESTS: CI Pipeline Validation
- [ ] **MDB003_6A_T1: Test Go tests run in CI**
- [ ] **MDB003_6A_T2: Test Go linting runs in CI**
- [ ] **MDB003_6A_T3: Test external tests run in CI**
- [ ] **MDB003_6A_T4: Test benchmarks run in CI**
- [ ] **MDB003_6A_T5: Test Docker build runs in CI**
- [ ] **MDB003_6A_T6: Test CI fails on test failures**
- [ ] **MDB003_6A_T7: Test CI badge updates**

### Phase 7: Documentation & Examples (Day 12-14)

#### Phase MDB003_7A: CODE: Documentation
- [ ] Create `docs/` directory
- [ ] Write `docs/README.md` - getting started guide
- [ ] Write `docs/API.md` - complete API reference
- [ ] Document all RPC methods with examples
- [ ] Write `docs/DEPLOYMENT.md` - production deployment
- [ ] Document Docker deployment
- [ ] Document Kubernetes deployment (optional)
- [ ] Write `docs/MIGRATION.md` - migration from Message DB
- [ ] Write `docs/PERFORMANCE.md` - performance tuning
- [ ] Create `docs/examples/` directory
- [ ] Write basic usage example
- [ ] Write optimistic locking example
- [ ] Write consumer groups example
- [ ] Write SSE subscriptions example
- [ ] Update root README.md with overview

#### Phase MDB003_7A: TESTS: Documentation Validation
- [ ] **MDB003_7A_T1: Test all code examples compile**
- [ ] **MDB003_7A_T2: Test deployment guide steps work**
- [ ] **MDB003_7A_T3: Test migration guide accurate**
- [ ] **MDB003_7A_T4: Test API examples match spec**
- [ ] **MDB003_7A_T5: Test links in documentation valid**

## Development Workflow Per Phase

For **EACH** phase:

1. **Implement Code** (Phase XA CODE)
2. **Write Tests IMMEDIATELY** (Phase XA TESTS)
3. **Run Tests & Verify** - All tests must pass (`bun test`)
4. **Validate Integration** - Test with real server
5. **Commit with good message** - Only if tests pass
6. **NEVER move to next phase with failing tests**

## File Structure

```
message-db/
├── test/
│   ├── package.json
│   ├── bun.lockb
│   ├── lib/
│   │   ├── client.ts              # MessageDB TypeScript client
│   │   └── helpers.ts             # Test utilities
│   ├── tests/
│   │   ├── stream.test.ts         # Stream operation tests
│   │   ├── category.test.ts       # Category operation tests
│   │   ├── subscription.test.ts   # SSE subscription tests
│   │   ├── namespace.test.ts      # Namespace isolation tests
│   │   └── concurrency.test.ts    # Concurrent operations tests
│   ├── benchmarks/
│   │   ├── performance.bench.ts   # Performance benchmarks
│   │   └── baseline.json          # Baseline metrics
│   └── fixtures/
│       └── test-data.json         # Test fixtures
├── docs/
│   ├── README.md                  # Getting started
│   ├── API.md                     # API reference
│   ├── DEPLOYMENT.md              # Deployment guide
│   ├── MIGRATION.md               # Migration guide
│   ├── PERFORMANCE.md             # Performance tuning
│   └── examples/
│       ├── basic-usage.md
│       ├── optimistic-locking.md
│       ├── consumer-groups.md
│       └── subscriptions.md
├── .github/
│   └── workflows/
│       ├── test.yml               # Main test workflow
│       ├── benchmark.yml          # Performance benchmarks
│       └── docker.yml             # Docker build/push
├── Dockerfile                     # Container image
├── docker-compose.yml             # Local development stack
└── Makefile                       # Build/test commands
```

## Code Size Estimates

```
test/lib/client.ts:         ~200 lines  (TypeScript client)
test/lib/helpers.ts:        ~100 lines  (Test utilities)
test/tests/*.test.ts:       ~800 lines  (All test scenarios)
test/benchmarks/*.ts:       ~150 lines  (Performance benchmarks)
docs/*.md:                  ~2000 lines (Documentation)
.github/workflows/*.yml:    ~200 lines  (CI/CD pipelines)
Dockerfile:                 ~30 lines   (Container image)
docker-compose.yml:         ~40 lines   (Dev stack)

Total test code:            ~1250 lines
Total documentation:        ~2000 lines
Total infrastructure:       ~270 lines
```

## Key Implementation Details

**TypeScript Client Pattern:**
```typescript
export class MessageDBClient {
  async rpc(method: string, ...args: any[]) {
    const response = await fetch(`${this.baseURL}/rpc`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': this.token ? `Bearer ${this.token}` : ''
      },
      body: JSON.stringify([method, ...args])
    });
    
    // Capture token from test mode
    const newToken = response.headers.get('X-MessageDB-Token');
    if (newToken && !this.token) {
      this.token = newToken;
    }
    
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error.code);
    }
    
    return response.json();
  }
}
```

**Test Server Pattern:**
```typescript
export async function startTestServer() {
  const proc = Bun.spawn(['./messagedb', 'serve', '--test-mode', '--port=0']);
  
  // Parse port from stdout
  const port = await new Promise<number>((resolve) => {
    const reader = proc.stdout.getReader();
    // Read until "Listening on :PORT" found
  });
  
  return {
    url: `http://localhost:${port}`,
    close: () => proc.kill()
  };
}
```

**Benchmark Pattern:**
```typescript
import { bench, run } from 'bun:test';

bench('stream write', async () => {
  await client.writeMessage('bench-stream', {
    type: 'Event',
    data: { value: Math.random() }
  });
});

await run(); // Returns performance metrics
```

## Test Distribution Summary

- **Phase 1 Tests:** 7 scenarios (Test client setup)
- **Phase 2 Tests:** 18 scenarios (Stream + category operations)
- **Phase 3 Tests:** 13 scenarios (Subscriptions + concurrency)
- **Phase 4 Tests:** 6 scenarios (Performance benchmarks)
- **Phase 5 Tests:** 6 scenarios (Docker validation)
- **Phase 6 Tests:** 7 scenarios (CI pipeline)
- **Phase 7 Tests:** 5 scenarios (Documentation validation)

**Total: 62 test scenarios covering all Epic MDB003 acceptance criteria**

## Dependencies

- **MDB002:** RPC API & Authentication complete
- **Bun.js 1.0+:** For test suite
- **Docker:** For containerization
- **GitHub repository:** For CI/CD

## Performance Targets

| Operation | Target | Test |
|-----------|--------|------|
| Stream write | <10ms p95 | MDB003_4A_T1 |
| Stream read (100) | <20ms p95 | MDB003_4A_T2 |
| Category read | <30ms p95 | MDB003_4A_T3 |
| SSE poke | <5ms | MDB003_4A_T4 |
| Throughput | >1000/sec | MDB003_4A_T5 |

---

## Implementation Status

### EPIC MDB003: TESTING & PRODUCTION READINESS - PENDING
### Current Status: READY FOR IMPLEMENTATION

### Progress Tracking
- [ ] Phase MDB003_1A: External Test Client
- [ ] Phase MDB003_2A: Stream Operation Tests
- [ ] Phase MDB003_2B: Category Operation Tests
- [ ] Phase MDB003_3A: SSE Subscription Tests
- [ ] Phase MDB003_3B: Concurrency & Isolation Tests
- [ ] Phase MDB003_4A: Performance Benchmarks
- [ ] Phase MDB003_5A: Docker & Deployment
- [ ] Phase MDB003_6A: CI/CD Pipeline
- [ ] Phase MDB003_7A: Documentation & Examples

### Definition of Done
- [ ] Bun.js test suite implemented
- [ ] TypeScript MessageDB client working
- [ ] All stream operation tests passing
- [ ] All category operation tests passing
- [ ] SSE subscription tests passing
- [ ] Namespace isolation verified
- [ ] Concurrent operation tests passing
- [ ] Performance benchmarks meet targets
- [ ] Dockerfile builds and runs
- [ ] Docker Compose working
- [ ] GitHub Actions CI pipeline working
- [ ] API documentation complete
- [ ] Deployment guide complete
- [ ] Migration guide complete
- [ ] All 62 test scenarios passing
- [ ] Ready for alpha release

### Important Rules
- ✅ Code compiles and tests pass before next phase
- ✅ Epic ID + test ID in test names (MDB003_XA_TN)
- ✅ Black-box testing only (no internal Go testing)
- ✅ Test mode uses SQLite in-memory
- ✅ Namespace isolation in all tests
- ✅ Performance targets must be met
- ✅ All documentation examples tested
- ✅ CI passes before merge
