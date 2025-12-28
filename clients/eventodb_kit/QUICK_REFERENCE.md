# EventodbKit + Code Generation - Quick Reference

## Setup (5 minutes)

### 1. Generate Event Schemas

```bash
cd eventodb-docs/code-gen
bun run index.ts elixir events.json lib/my_app/events/
```

### 2. Create Event Registry

```elixir
# lib/my_app/events.ex
defmodule MyApp.Events do
  use EventodbKit.EventDispatcher
  
  register "UserCreated", MyApp.Events.UserCreated
  register "OrderPlaced", MyApp.Events.OrderPlaced
  # ... register all your events
end
```

### 3. Use in Consumer

```elixir
defmodule MyApp.MyConsumer do
  use EventodbKit.Consumer
  
  def handle_message(message, state) do
    case MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, _} -> :ok
      {:error, :unknown_event} -> :ok
      {:error, _changeset} -> {:error, :validation_failed}
    end
  end
  
  defp handle_event(MyApp.Events.UserCreated, event) do
    # Your logic here
    :ok
  end
end
```

## Common Patterns

### Pattern Matching Handler

```elixir
defp handle_event(MyApp.Events.UserCreated, event) do
  create_profile(event.user_id, event.email)
  :ok
end

defp handle_event(MyApp.Events.OrderPlaced, event) do
  process_order(event.order_id)
  :ok
end

defp handle_event(_module, _event), do: :ok
```

### Error Handling

```elixir
case MyApp.Events.dispatch(type, data, &handle_event/2) do
  {:ok, result} -> 
    Logger.info("Processed: #{type}")
    :ok
  
  {:error, :unknown_event} -> 
    Logger.debug("Unknown: #{type}")
    :ok
  
  {:error, changeset} -> 
    Logger.error("Validation failed: #{inspect(changeset.errors)}")
    {:error, :validation_failed}
end
```

### Validate Before Publishing

```elixir
{:ok, validated} = MyApp.Events.validate("UserCreated", params)

EventodbKit.Outbox.insert(
  repo,
  namespace,
  MyApp.Events.UserCreated.stream_name(validated),
  "UserCreated",
  Map.from_struct(validated)
)
```

### Transactional Publishing

```elixir
Repo.transaction(fn ->
  # Business operation
  user = Repo.insert!(%User{email: "test@example.com"})
  
  # Validate event
  {:ok, validated} = MyApp.Events.validate("UserCreated", %{
    user_id: user.id,
    email: user.email
  })
  
  # Add to outbox
  EventodbKit.Outbox.insert(
    Repo,
    namespace,
    MyApp.Events.UserCreated.stream_name(validated),
    "UserCreated",
    Map.from_struct(validated)
  )
end)
```

## API Cheat Sheet

### EventDispatcher

```elixir
# Dispatch with validation
MyApp.Events.dispatch(type, data, handler)
# => {:ok, result} | {:error, :unknown_event} | {:error, changeset}

# Validate only
MyApp.Events.validate(type, data)
# => {:ok, struct} | {:error, :unknown_event} | {:error, changeset}

# List registered events
MyApp.Events.registered_events()
# => ["UserCreated", "OrderPlaced", ...]

# Get event module
MyApp.Events.event_module("UserCreated")
# => MyApp.Events.UserCreated

# Get event category
MyApp.Events.event_category("UserCreated")
# => {:ok, "user"}
```

### Generated Event Module

```elixir
# Validate
Events.UserCreated.validate!(data)
# => {:ok, %Events.UserCreated{}} | {:error, changeset}

# Get category
Events.UserCreated.category()
# => "user"

# Build stream name
Events.UserCreated.stream_name(%{user_id: "123"})
# => "user-123"

# Create changeset
Events.UserCreated.changeset(data)
# => %Ecto.Changeset{}
```

## Testing

### Unit Test

```elixir
test "handles UserCreated" do
  result = MyApp.Events.dispatch("UserCreated", %{
    user_id: "123",
    email: "test@example.com"
  }, fn module, event ->
    assert module == MyApp.Events.UserCreated
    assert event.user_id == "123"
    :processed
  end)
  
  assert result == {:ok, :processed}
end
```

### Integration Test

```elixir
test "consumes events", %{kit: kit} do
  {:ok, _, _} = EventodbEx.stream_write(
    kit.eventodb_client,
    "user-123",
    %{type: "UserCreated", data: %{user_id: "123", email: "test@example.com"}}
  )
  
  assert_receive {:processed, result}, 1000
end
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Unknown event error | Register event in dispatcher module |
| Validation fails | Check required fields and types |
| Event not processed | Verify category matches consumer |
| Wrong data received | Check message format from EventoDB |

## Best Practices

✅ **DO:**
- Pattern match on event modules
- Validate before publishing
- Handle unknown events gracefully
- Use separate registries per context
- Log validation errors

❌ **DON'T:**
- String match event types
- Skip validation
- Fail consumer on unknown events
- Mix unrelated events in one registry
- Ignore validation errors

## Full Documentation

- **[CODEGEN_API.md](CODEGEN_API.md)** - Complete integration guide
- **[README.md](README.md)** - EventodbKit overview
- **[CODEGEN_INTEGRATION_SUMMARY.md](CODEGEN_INTEGRATION_SUMMARY.md)** - Implementation details
