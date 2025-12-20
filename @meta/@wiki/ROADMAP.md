# EventoDB Go - Roadmap

**Project:** EventoDB Go Server  
**Status:** Planning Phase  
**Last Updated:** 2024-12-17

---

## Vision

Build a production-ready HTTP API server that wraps EventoDB, providing simple RPC-style access to event sourcing capabilities with multi-tenant namespace support.

---

## Epic Overview

```
Epic 1: Core Storage & Migrations (Foundation)
    │
    ├─> Epic 2: RPC API & Authentication (Interface)
    │
    └─> Epic 3: Testing & Production Readiness (Quality)
```

---

## Epic 1: Core Storage & Migrations

**Goal:** Establish reliable, dual-backend storage layer with automatic migrations

**Duration:** 2-3 weeks  
**Dependencies:** None  
**Deliverables:**
- Store interface defined
- Postgres backend working (with stored procedures)
- SQLite backend working (Go-based logic)
- Auto-migration system on boot
- Namespace creation/deletion
- Physical data isolation (schemas/databases per namespace)

**Key Components:**
- `internal/store/store.go` - Store interface
- `internal/store/postgres/` - Postgres implementation
- `internal/store/sqlite/` - SQLite implementation
- `internal/migrate/` - Migration engine
- `migrations/metadata/` - Namespace registry migrations
- `migrations/namespace/` - EventoDB structure templates

**Success Criteria:**
- Can create/delete namespaces
- Can write/read messages to/from streams
- Can query categories with consumer groups
- Migrations run automatically on startup
- Both backends pass same test suite
- Existing EventoDB functions all accessible

---

## Epic 2: RPC API & Authentication

**Goal:** HTTP server with RPC endpoints and token-based namespace authentication

**Duration:** 2-3 weeks  
**Dependencies:** Epic 1 complete  
**Deliverables:**
- RPC handler (`POST /rpc`)
- Token generation and validation
- Authentication middleware
- SSE subscription endpoint (`GET /subscribe`)
- Namespace management API
- Test mode support

**Key Components:**
- `internal/api/handler.go` - RPC request routing
- `internal/api/sse.go` - Server-Sent Events
- `internal/api/middleware.go` - Auth middleware
- `internal/auth/` - Token generation/validation
- `cmd/eventodb/main.go` - Server binary

**API Methods:**
- `stream.write`, `stream.get`, `stream.last`, `stream.version`
- `category.get`
- `ns.create`, `ns.delete`, `ns.list`, `ns.info`
- `sys.version`, `sys.health`

**Success Criteria:**
- All RPC methods work via HTTP
- Token authentication enforces namespace isolation
- SSE subscriptions deliver pokes
- Test mode auto-creates namespaces
- Error responses follow spec
- Can run multiple namespaces concurrently

---

## Epic 3: Testing & Production Readiness

**Goal:** Comprehensive test coverage and production deployment capability

**Duration:** 1-2 weeks  
**Dependencies:** Epic 2 complete  
**Deliverables:**
- External Bun.js test suite
- Performance benchmarks
- Documentation and examples
- Docker/deployment setup
- CI/CD pipeline
- Migration from existing EventoDB guide

**Key Components:**
- `test/` - Bun.js black-box tests
- `test/tests/*.test.ts` - Test scenarios
- `test/lib/client.ts` - TypeScript client
- `docs/` - User documentation
- `examples/` - Usage examples
- `.github/workflows/` - CI pipeline
- `Dockerfile` - Container image

**Test Coverage:**
- Stream operations (write, read, version)
- Category queries with consumer groups
- Namespace isolation
- Concurrent operations
- Optimistic locking
- SSE subscriptions
- Error handling
- Migration compatibility

**Success Criteria:**
- All external tests pass
- Can import and run existing EventoDB tests
- Performance within 20% of direct Postgres access
- Complete API documentation
- Docker image builds and runs
- CI runs tests on every commit
- Ready for alpha release

---

## Milestones

| Milestone | Target | Status |
|-----------|--------|--------|
| **M1:** Epic 1 Complete - Storage Working | Week 3 | 🔴 Not Started |
| **M2:** Epic 2 Complete - API Working | Week 6 | 🔴 Not Started |
| **M3:** Epic 3 Complete - Production Ready | Week 8 | 🔴 Not Started |
| **M4:** Alpha Release | Week 9 | 🔴 Not Started |

---

## Release Plan

### v0.1.0-alpha (After Epic 3)
- Core functionality complete
- Internal testing only
- Documentation in progress

### v0.2.0-beta (After feedback)
- Production testing with real workloads
- Performance optimization
- API refinements

### v1.0.0 (Stable)
- Production-ready
- Full documentation
- Migration guide from EventoDB
- Client libraries

---

## Out of Scope (Future Versions)

- Clustering / High Availability
- Built-in metrics/observability (use external tools)
- Rate limiting / Quotas
- Backup/restore tooling
- Message replay features
- GraphQL interface
- WebSocket support (SSE sufficient)

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance overhead vs direct Postgres | High | Benchmark early, optimize hot paths |
| SQLite limitations for tests | Medium | Document limitations, Postgres for production |
| Migration complexity (template-based) | Medium | Thorough testing, rollback support |
| Token management (lost tokens) | Low | Document backup procedures |
| Namespace deletion accidents | Medium | Require explicit confirmation, admin-only |

---

## Team Structure

**Solo Developer:** 1 person
- Full-stack (Go backend + TypeScript tests)
- Infrastructure (Postgres + Docker)
- Documentation

**Recommended Tools:**
- Go 1.21+
- Postgres 14+
- Bun.js for tests
- VSCode / GoLand
- Docker Desktop

---

## Success Metrics

**Technical:**
- Test coverage > 80%
- API response time < 50ms (p95)
- Can handle 1000+ writes/sec per namespace
- Zero data leaks between namespaces

**Developer Experience:**
- Setup time < 5 minutes
- API intuitive (measured by example complexity)
- Error messages actionable

**Production:**
- Can migrate existing EventoDB instance
- Can run in Docker/K8s
- Monitoring integration straightforward

---

## Next Steps

1. ✅ Complete ADRs (Done)
2. ✅ Create roadmap (This document)
3. 🔲 Create Epic 1 detailed plan
4. 🔲 Start implementation - Epic 1

---

## References

- ADRs: `@meta/@adr/`
- Design Doc: `@meta/DESIGN.md`
- EventoDB Repo: https://github.com/eventodb/eventodb
