# EventoDBEx

Elixir client for EventoDB - a simple, fast message store.

## Installation

Add `eventodb_ex` to your list of dependencies in `mix.exs`:

```elixir
def deps do
  [
    {:eventodb_ex, "~> 0.1.0"}
  ]
end
```

## Quick Start

```elixir
# Create a client
client = EventodbEx.Client.new("http://localhost:8080", token: "ns_...")

# Write a message
{:ok, result, client} = EventodbEx.stream_write(
  client,
  "account-123",
  %{type: "Deposited", data: %{amount: 100}}
)

IO.puts("Written at position: #{result.position}")

# Read messages from stream
{:ok, messages, client} = EventodbEx.stream_get(client, "account-123")

Enum.each(messages, fn [id, type, position, global_position, data, metadata, time] ->
  IO.puts("Message #{id}: #{type} at position #{position}")
end)
```

## Features

- ✅ Stream operations (write, read, last, version)
- ✅ Category operations with consumer groups and correlation
- ✅ Namespace management
- ✅ System health and version
- ✅ Optimistic locking with expected version
- ✅ Minimal dependencies (just `req` and `jason`)
- ✅ Idiomatic Elixir patterns with pattern matching
- ✅ Comprehensive test suite

## Usage

### Creating a Client

```elixir
# With token
client = EventodbEx.Client.new("http://localhost:8080", token: "ns_...")

# Without token (will auto-capture in test mode)
client = EventodbEx.Client.new("http://localhost:8080")
```

### Writing Messages

```elixir
# Simple write
{:ok, result, client} = EventodbEx.stream_write(
  client,
  "account-123",
  %{
    type: "Deposited",
    data: %{amount: 100}
  }
)

# With metadata
{:ok, result, client} = EventodbEx.stream_write(
  client,
  "account-123",
  %{
    type: "Deposited",
    data: %{amount: 100},
    metadata: %{
      correlationStreamName: "workflow-456",
      causationMessageId: "msg-123"
    }
  }
)

# With optimistic locking
{:ok, result, client} = EventodbEx.stream_write(
  client,
  "account-123",
  %{type: "Deposited", data: %{amount: 100}},
  %{expected_version: 5}
)
```

### Reading Messages

```elixir
# Read all messages
{:ok, messages, client} = EventodbEx.stream_get(client, "account-123")

# Read from position
{:ok, messages, client} = EventodbEx.stream_get(
  client,
  "account-123",
  %{position: 10}
)

# Read with batch size
{:ok, messages, client} = EventodbEx.stream_get(
  client,
  "account-123",
  %{batch_size: 100}
)

# Pattern match on message structure
{:ok, messages, client} = EventodbEx.stream_get(client, "account-123")

Enum.each(messages, fn [id, type, pos, gpos, data, metadata, time] ->
  # Process message
end)
```

### Last Message

```elixir
# Get last message
{:ok, message, client} = EventodbEx.stream_last(client, "account-123")

# Get last message of specific type
{:ok, message, client} = EventodbEx.stream_last(
  client,
  "account-123",
  %{type: "Deposited"}
)
```

### Stream Version

```elixir
{:ok, version, client} = EventodbEx.stream_version(client, "account-123")
# version is 0-based, so version 5 means 6 messages (positions 0-5)
```

### Category Operations

```elixir
# Read all messages in a category
{:ok, messages, client} = EventodbEx.category_get(client, "account")

# With consumer group (for scaling)
{:ok, messages, client} = EventodbEx.category_get(
  client,
  "account",
  %{
    consumer_group: %{
      member: 0,  # This consumer's index
      size: 4     # Total number of consumers
    }
  }
)

# With correlation filter
{:ok, messages, client} = EventodbEx.category_get(
  client,
  "account",
  %{correlation: "workflow"}
)

# Category messages have 8 elements (includes streamName)
{:ok, [msg], client} = EventodbEx.category_get(client, "account")
[id, stream_name, type, pos, gpos, data, metadata, time] = msg
```

### Namespace Management

```elixir
# Create namespace
{:ok, result, client} = EventodbEx.namespace_create(
  client,
  "my-app",
  %{description: "My application namespace"}
)

# Use the token
client = EventodbEx.Client.set_token(client, result.token)

# List namespaces
{:ok, namespaces, client} = EventodbEx.namespace_list(client)

# Get namespace info
{:ok, info, client} = EventodbEx.namespace_info(client, "my-app")

# Delete namespace (⚠️ irreversible!)
{:ok, result, client} = EventodbEx.namespace_delete(client, "my-app")
```

### System Operations

```elixir
# Get server version
{:ok, version, client} = EventodbEx.system_version(client)

# Get health status
{:ok, health, client} = EventodbEx.system_health(client)
```

## Error Handling

The SDK uses Elixir's standard `{:ok, result}` and `{:error, error}` tuples:

```elixir
case EventodbEx.stream_write(client, stream, message, %{expected_version: 5}) do
  {:ok, result, client} ->
    # Success
    IO.puts("Written at position #{result.position}")
    
  {:error, %EventodbEx.Error{code: "STREAM_VERSION_CONFLICT"}} ->
    # Handle conflict
    IO.puts("Version conflict - retry")
    
  {:error, error} ->
    # Other error
    IO.puts("Error: #{error.message}")
end
```

Common error codes:
- `AUTH_REQUIRED` - No authentication token
- `AUTH_INVALID` - Invalid token
- `STREAM_VERSION_CONFLICT` - Optimistic locking conflict
- `NAMESPACE_EXISTS` - Namespace already exists
- `NAMESPACE_NOT_FOUND` - Namespace doesn't exist
- `NETWORK_ERROR` - Connection or network issue

## Testing

Tests run against a live EventoDB server. Each test creates its own namespace for isolation.

```bash
# Start EventoDB server
docker-compose up -d

# Run tests
mix test

# With custom server URL
EVENTODB_URL=http://localhost:8080 mix test

# Run specific test file
mix test test/write_test.exs

# Run with coverage
mix test --cover
```

## Message Format

### Stream Messages

Stream messages are 7-element lists:

```elixir
[
  id,              # String - Message UUID
  type,            # String - Event type
  position,        # Integer - Stream position (0-based)
  global_position, # Integer - Global sequence number
  data,            # Map - Event payload
  metadata,        # Map or nil - Message metadata
  time             # String - ISO 8601 timestamp (UTC)
]
```

### Category Messages

Category messages are 8-element lists (includes stream name):

```elixir
[
  id,              # String - Message UUID
  stream_name,     # String - Full stream name
  type,            # String - Event type
  position,        # Integer - Stream position (0-based)
  global_position, # Integer - Global sequence number
  data,            # Map - Event payload
  metadata,        # Map or nil - Message metadata
  time             # String - ISO 8601 timestamp (UTC)
]
```

## Advanced Patterns

### Consumer Groups

Distribute category processing across multiple consumers:

```elixir
# Consumer 1
{:ok, messages, client} = EventodbEx.category_get(
  client,
  "account",
  %{consumer_group: %{member: 0, size: 4}}
)

# Consumer 2
{:ok, messages, client} = EventodbEx.category_get(
  client,
  "account",
  %{consumer_group: %{member: 1, size: 4}}
)

# Each consumer gets a deterministic subset of streams
```

### Optimistic Locking

Prevent concurrent write conflicts:

```elixir
# Read current version
{:ok, version, client} = EventodbEx.stream_version(client, "account-123")

# Write with expected version
case EventodbEx.stream_write(
  client,
  "account-123",
  message,
  %{expected_version: version}
) do
  {:ok, result, client} -> {:ok, result, client}
  {:error, %{code: "STREAM_VERSION_CONFLICT"}} -> retry()
end
```

### Correlation

Track related messages across streams:

```elixir
# Write with correlation
{:ok, _, client} = EventodbEx.stream_write(
  client,
  "account-123",
  %{
    type: "Deposited",
    data: %{amount: 100},
    metadata: %{correlationStreamName: "workflow-456"}
  }
)

# Query by correlation
{:ok, messages, client} = EventodbEx.category_get(
  client,
  "account",
  %{correlation: "workflow"}
)
```

## License

MIT - see [LICENSE](LICENSE)

## Links

- [EventoDB GitHub](https://github.com/eventodb-hq/eventodb)
- [EventoDB API Documentation](https://github.com/eventodb-hq/eventodb/blob/main/docs/API.md)
- [Req HTTP Client](https://github.com/wojtekmach/req)
