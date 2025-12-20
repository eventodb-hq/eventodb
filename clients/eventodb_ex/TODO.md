# Elixir SDK TODO

## ✅ Completed

- [x] **SSE Subscriptions** (10 tests passing!)
  - [x] Implement `subscribe_to_stream/3`
  - [x] Implement `subscribe_to_category/3`
  - [x] SSE parser for poke events
  - [x] GenServer with Registry for named subscriptions
  - [x] Add SSE tests (SSE-001 through SSE-008 + extras)

## Important

- [ ] **Edge Case Tests** (3 tests)
  - [ ] EDGE-005: Concurrent writes test
  - [ ] EDGE-007: Expected version -1 test
  - [ ] EDGE-008: Expected version 0 test

## Nice to Have

- [ ] **Additional Edge Tests** (5 tests)
  - [ ] EDGE-001: Empty batch size
  - [ ] EDGE-002: Negative position
  - [ ] EDGE-003: Very large batch size
  - [ ] EDGE-004: Stream name edge cases
  - [ ] EDGE-006: Read beyond stream end

- [ ] **Documentation**
  - [ ] SSE status/workarounds in README
  - [ ] Optimistic concurrency examples
  - [ ] Error code reference
  - [ ] Polling patterns

- [ ] **Tests**
  - [ ] NS-002: Custom token (optional)

## Notes

- ERROR-002/003: Compile-time checks, no runtime test needed
- ERROR-005/006/007: HTTP library handles, document only
