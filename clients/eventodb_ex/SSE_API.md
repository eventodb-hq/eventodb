# SSE API for Elixir SDK

## API

```elixir
# Subscribe to stream
{:ok, subscription} = EventodbEx.subscribe_to_stream(client, stream_name, opts)

# Subscribe to category  
{:ok, subscription} = EventodbEx.subscribe_to_category(client, category, opts)

# Close
EventodbEx.Subscription.close(subscription)
```

## Options

```elixir
opts = [
  position: 0,                              # Starting position
  consumer_group: %{member: 0, size: 4},   # Category only
  on_poke: fn poke -> ... end,             # Required
  on_error: fn error -> ... end            # Optional
]
```

## Poke Event

```elixir
%{
  stream: "account-123",
  position: 5,
  global_position: 1234
}
```

## Example

```elixir
{:ok, sub} = EventodbEx.subscribe_to_category(client, "account",
  on_poke: fn poke ->
    IO.puts("New message: #{poke.stream}@#{poke.position}")
  end
)

# Later...
EventodbEx.Subscription.close(sub)
```

## Implementation

1. Add `mint` dependency for SSE streaming
2. Create `EventodbEx.Subscription` GenServer
3. Parse SSE `event: poke` and `data: {...}` format
4. Call `on_poke` callback for each event

See `SSE_PROPOSAL.md` for full implementation.
