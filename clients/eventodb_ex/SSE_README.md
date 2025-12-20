# Real-time Subscriptions (SSE)

Subscribe to real-time message notifications using Server-Sent Events.

## Quick Start

```elixir
# Subscribe to a stream
{:ok, _pid} = EventodbEx.subscribe_to_stream(client, "account-123",
  name: "my-account-sub",
  on_poke: fn poke ->
    IO.puts("New message: #{poke.stream}@#{poke.position}")
  end
)

# Close subscription
EventodbEx.Subscription.close("my-account-sub")
```

## API

### subscribe_to_stream/3

Subscribe to a single stream.

```elixir
EventodbEx.subscribe_to_stream(client, stream_name, opts)
```

**Options:**
- `name:` (required) - String name for the subscription
- `position:` (optional) - Starting position, default: 0
- `on_poke:` (required) - Callback `fn poke -> ... end`
- `on_error:` (optional) - Error callback `fn error -> ... end`

**Returns:** `{:ok, pid}` or `{:error, reason}`

### subscribe_to_category/3

Subscribe to all streams in a category.

```elixir
EventodbEx.subscribe_to_category(client, category_name, opts)
```

**Options:**
- `name:` (required) - String name for the subscription
- `position:` (optional) - Starting global position, default: 0
- `consumer_group:` (optional) - `%{member: 0, size: 4}` for partitioning
- `on_poke:` (required) - Callback `fn poke -> ... end`
- `on_error:` (optional) - Error callback `fn error -> ... end`

**Returns:** `{:ok, pid}` or `{:error, reason}`

### Subscription.close/1

Close a subscription by name.

```elixir
EventodbEx.Subscription.close(name)
```

**Returns:** `:ok` or `{:error, :not_found}`

## Poke Event

```elixir
%{
  stream: "account-123",        # Stream name
  position: 5,                  # Position in stream
  global_position: 1234         # Global sequence number
}
```

## Examples

### Simple processor

```elixir
{:ok, _pid} = EventodbEx.subscribe_to_stream(client, "account-123",
  name: "account-processor",
  on_poke: fn poke ->
    # Fetch the new message
    {:ok, messages, _} = EventodbEx.stream_get(client, poke.stream,
      position: poke.position,
      batch_size: 1
    )
    
    # Process it
    Enum.each(messages, &process_message/1)
  end
)
```

### Category with consumer group

```elixir
# Worker 1 (handles streams hashed to partition 0)
{:ok, _pid} = EventodbEx.subscribe_to_category(client, "account",
  name: "worker-1",
  consumer_group: %{member: 0, size: 4},
  on_poke: fn poke ->
    {:ok, messages, _} = EventodbEx.category_get(client, "account",
      position: poke.global_position,
      consumer_group: %{member: 0, size: 4}
    )
    process_messages(messages)
  end
)

# Worker 2-4 use member: 1, 2, 3 respectively
```

### GenServer processor with checkpointing

```elixir
defmodule MyProcessor do
  use GenServer

  def start_link(client, category) do
    GenServer.start_link(__MODULE__, {client, category}, name: __MODULE__)
  end

  def init({client, category}) do
    # Load checkpoint from DB
    last_position = load_checkpoint() || 0
    
    {:ok, _pid} = EventodbEx.subscribe_to_category(client, category,
      name: "#{category}-processor",
      position: last_position,
      on_poke: fn poke -> send(__MODULE__, {:poke, poke}) end
    )
    
    {:ok, %{client: client, category: category, last_position: last_position}}
  end

  def handle_info({:poke, poke}, state) do
    # Fetch new messages
    {:ok, messages, client} = EventodbEx.category_get(
      state.client,
      state.category,
      position: state.last_position + 1
    )
    
    # Process each message
    Enum.each(messages, &process_message/1)
    
    # Update checkpoint
    new_position = get_max_global_position(messages)
    save_checkpoint(new_position)
    
    {:noreply, %{state | client: client, last_position: new_position}}
  end

  def terminate(_reason, state) do
    EventodbEx.Subscription.close("#{state.category}-processor")
  end
end
```

### Error handling

```elixir
{:ok, _pid} = EventodbEx.subscribe_to_category(client, "account",
  name: "resilient-processor",
  on_poke: fn poke ->
    try do
      process_poke(poke)
    rescue
      e -> Logger.error("Failed to process poke: #{inspect(e)}")
    end
  end,
  on_error: fn error ->
    Logger.error("SSE error: #{inspect(error)}")
    # Maybe restart subscription
  end
)
```

## Position Tracking

**You must track the last processed position yourself!**

The subscription only notifies you of new messages - it doesn't track what you've processed.

```elixir
# Bad - will reprocess everything on restart
{:ok, _} = EventodbEx.subscribe_to_category(client, "account",
  name: "processor",
  on_poke: fn poke -> process(poke) end
)

# Good - resume from last checkpoint
last_position = load_checkpoint() || 0

{:ok, _} = EventodbEx.subscribe_to_category(client, "account",
  name: "processor",
  position: last_position,  # Resume from here
  on_poke: fn poke ->
    process(poke)
    save_checkpoint(poke.global_position)
  end
)
```

## Implementation Details

- Uses `Mint.HTTP` for SSE streaming
- Each subscription is a GenServer
- Subscriptions are registered in Registry with string names
- Connection errors call `on_error` callback
- SSE events are parsed and converted to poke structs
- No automatic reconnection (handle in your `on_error` callback)

## Testing

Run SSE tests:

```bash
mix test test/sse_test.exs
```

All 10 SSE tests pass including:
- Stream and category subscriptions
- Position filtering
- Consumer groups
- Multiple subscriptions
- Named subscription management
