# Changelog

## v1.0.0 - 2024-12-20

### Initial Release

Complete implementation of the EventoDB Go SDK with full SDK test spec compliance.

#### Features

- **Stream Operations**
  - `StreamWrite` - Write messages to streams with optional expected version
  - `StreamGet` - Read messages with position/global position filtering and batch size control
  - `StreamLast` - Get the last message from a stream, optionally filtered by type
  - `StreamVersion` - Get the current version of a stream

- **Category Operations**
  - `CategoryGet` - Read from categories with:
    - Position-based filtering
    - Batch size control
    - Consumer group support for distributed processing
    - Correlation filtering

- **Namespace Operations**
  - `NamespaceCreate` - Create new namespaces with optional custom tokens
  - `NamespaceDelete` - Delete namespaces and all their data
  - `NamespaceList` - List all accessible namespaces
  - `NamespaceInfo` - Get metadata and message count for a namespace

- **System Operations**
  - `SystemVersion` - Get server version
  - `SystemHealth` - Check server health status

- **Server-Sent Events (SSE)**
  - `SubscribeStream` - Subscribe to real-time poke events for a stream
  - `SubscribeCategory` - Subscribe to poke events for a category
  - Support for position-based subscriptions
  - Consumer group support for load-balanced subscriptions
  - Proper cleanup and resource management

#### Implementation Details

- **Zero Dependencies**: Uses only Go standard library
- **Context-Aware**: All operations accept `context.Context` for cancellation and timeouts
- **Type-Safe**: Strongly typed message structures with proper Go idioms
- **Error Handling**: Semantic error types with `errors.Is` support
- **Functional Options**: Idiomatic configuration pattern for client and operations
- **Concurrent-Safe**: All tests pass with race detector enabled

#### Test Coverage

- **71 tests** covering all SDK spec requirements
- **100% Tier 1** (Must Have): WRITE, READ, AUTH, ERROR
- **100% Tier 2** (Should Have): CATEGORY, NS, SYS, LAST, VERSION
- **100% Tier 3** (Nice to Have): ENCODING, SSE

#### Files

- `client.go` (9.9K) - Main client implementation
- `types.go` (3.1K) - Type definitions
- `errors.go` (1.2K) - Error types
- `sse.go` (5.1K) - SSE subscription support
- Test files (40K total) - Comprehensive test coverage
- Documentation (12.8K) - README, TESTING, and CHANGELOG

#### Known Limitations

- Global position filtering behavior varies in namespace-isolated environments
- Some server configurations allow unauthenticated access in test/dev mode
- Namespace message counts may be eventually consistent
