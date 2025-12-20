# Quick Start Guide

## Prerequisites

1. EventoDB server running on `http://localhost:8080`
2. Elixir 1.18+ installed

## Installation

```bash
cd clients/eventodb_ex
mix deps.get
mix compile
```

## Running Tests

```bash
# Make sure EventoDB server is running
# The server should be in test mode or have auth disabled for namespace creation

# Run all tests
mix test

# Run specific test category
mix test test/write_test.exs
mix test test/read_test.exs

# Run with custom server URL
EVENTODB_URL=http://localhost:9000 mix test

# Verbose output
mix test --trace
```

## Test Coverage

The SDK implements the following test tiers from `docs/SDK-TEST-SPEC.md`:

### Tier 1 (Must Have) ✅
- WRITE-001 through WRITE-010: Stream writing operations
- READ-001 through READ-010: Stream reading operations  
- AUTH-001 through AUTH-004: Authentication
- ERROR-001 through ERROR-004: Error handling

### Tier 2 (Should Have) ✅
- LAST-001 through LAST-004: Last message operations
- VERSION-001 through VERSION-003: Stream version
- CATEGORY-001 through CATEGORY-008: Category operations
- NS-001 through NS-008: Namespace management
- SYS-001, SYS-002: System operations

### Tier 3 (Nice to Have) ✅
- ENCODING-001 through ENCODING-010: Data encoding and special characters
- EDGE tests: Skipped (implementation-specific)
- SSE tests: Not implemented (requires GenServer/subscription handling)

## Example Usage

```elixir
# In iex
iex> client = EventodbEx.Client.new("http://localhost:8080")

# Create a namespace
iex> {:ok, ns, client} = EventodbEx.namespace_create(client, "my-app")

# Write a message
iex> message = %{type: "UserCreated", data: %{email: "user@example.com"}}
iex> {:ok, result, client} = EventodbEx.stream_write(client, "user-123", message)
iex> result.position
0

# Read it back
iex> {:ok, [msg], client} = EventodbEx.stream_get(client, "user-123")
iex> [id, type, pos, gpos, data, metadata, time] = msg
iex> type
"UserCreated"
iex> data
%{"email" => "user@example.com"}
```

## Development

### Code Structure

- `lib/eventodb_ex.ex` - Main public API
- `lib/eventodb_ex/client.ex` - HTTP/RPC client
- `lib/eventodb_ex/types.ex` - Type specifications
- `lib/eventodb_ex/error.ex` - Error handling
- `test/` - Comprehensive test suite

### Design Principles

1. **Minimal** - Only required dependencies (req, jason)
2. **Idiomatic** - Uses Elixir patterns (pattern matching, tuples, pipelines)
3. **Clean** - Clear separation of concerns
4. **Tested** - Comprehensive test coverage against live server

### Adding Features

The SDK is intentionally minimal. For advanced features like:
- SSE subscriptions: Would add GenServer-based subscription manager
- Connection pooling: Already handled by Req/Finch
- Retry logic: Can configure at Req level
- Batched writes: Add helper functions wrapping multiple writes

## Troubleshooting

### Tests fail with "AUTH_REQUIRED"

Make sure the EventoDB server is running in test mode or has auth disabled for namespace creation.

### Connection refused errors

Check that the server is running on the expected URL:
```bash
curl http://localhost:8080/health
```

### Tests create too many namespaces

Tests automatically clean up namespaces in the `on_exit` callback. If tests are interrupted, you may need to manually clean up:

```elixir
client = EventodbEx.Client.new("http://localhost:8080")
{:ok, namespaces, _} = EventodbEx.namespace_list(client)

# Delete test namespaces
Enum.each(namespaces, fn ns ->
  if String.starts_with?(ns.namespace, "test-") do
    EventodbEx.namespace_delete(client, ns.namespace)
  end
end)
```

## Next Steps

1. Review the [full README](README.md) for detailed API documentation
2. Check [SDK Test Spec](../../docs/SDK-TEST-SPEC.md) for test requirements
3. See [API Reference](../../docs/API.md) for server API details
4. Look at test files for usage examples
