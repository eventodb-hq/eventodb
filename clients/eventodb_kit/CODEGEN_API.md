# EventoDB Kit - Code Generation Integration

Clean, type-safe integration of generated event schemas from `eventodb-docs/code-gen`.

## Quick Start

### 1. Generate Event Schemas

```bash
cd eventodb-docs/code-gen
bun run index.ts elixir path/to/events.json output/directory/
```

This generates Ecto schemas with validation for each event.

### 2. Create Event Registry

```elixir
defmodule MyApp.Events do
  use EventodbKit.EventDispatcher
  
  # Register each generated event module
  register "UserCreated", MyApp.Events.UserCreated
  register "UserUpdated", MyApp.Events.UserUpdated
  register "OrderPlaced", MyApp.Events.OrderPlaced
  register "PaymentProcessed", MyApp.Events.PaymentProcessed
end
```

### 3. Use in Consumers

```elixir
defmodule MyApp.UserEventConsumer do
  use EventodbKit.Consumer
  
  def handle_message(message, state) do
    # Dispatch with automatic validation
    case MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, _result} -> :ok
      {:error, :unknown_event} -> :ok  # Skip unknown events
      {:error, changeset} -> {:error, :validation_failed}
    end
  end
  
  # Pattern match on event modules for type safety
  defp handle_event(MyApp.Events.UserCreated, event) do
    # event is a validated struct with all fields
    create_user_profile(event.user_id, event.email)
    :ok
  end
  
  defp handle_event(MyApp.Events.UserUpdated, event) do
    update_user_profile(event.user_id, event)
    :ok
  end
end
```

## API Reference

### EventodbKit.EventDispatcher

**`use EventodbKit.EventDispatcher`**

Creates an event registry module with the following functions:

#### `dispatch(event_type, data, handler)`

Validates and dispatches an event to a handler function.

**Parameters:**
- `event_type` (String) - Event type name (e.g., "UserCreated")
- `data` (Map) - Event payload data
- `handler` (function) - 2-arity function `(event_module, validated_event) -> result`

**Returns:**
- `{:ok, result}` - Handler executed successfully
- `{:error, :unknown_event}` - Event type not registered
- `{:error, changeset}` - Validation failed

**Example:**
```elixir
MyApp.Events.dispatch("UserCreated", %{"user_id" => "123"}, fn module, event ->
  IO.puts("User: #{event.user_id}")
  :processed
end)
# => {:ok, :processed}
```

#### `validate(event_type, data)`

Validates event data without dispatching.

**Parameters:**
- `event_type` (String) - Event type name
- `data` (Map) - Event payload data

**Returns:**
- `{:ok, validated_struct}` - Validation successful
- `{:error, :unknown_event}` - Event type not registered
- `{:error, changeset}` - Validation failed

**Example:**
```elixir
MyApp.Events.validate("UserCreated", %{
  "user_id" => "123",
  "email" => "user@example.com"
})
# => {:ok, %MyApp.Events.UserCreated{user_id: "123", email: "user@example.com"}}
```

#### `registered_events()`

Returns list of all registered event types.

**Example:**
```elixir
MyApp.Events.registered_events()
# => ["UserCreated", "UserUpdated", "OrderPlaced", ...]
```

#### `event_module(event_type)`

Returns the event module for a given type.

**Example:**
```elixir
MyApp.Events.event_module("UserCreated")
# => MyApp.Events.UserCreated
```

#### `event_category(event_type)`

Returns the event category for a given type.

**Example:**
```elixir
MyApp.Events.event_category("UserCreated")
# => {:ok, "user"}
```

### EventodbKit.EventDispatcher.publish_event/5

Helper function for publishing validated events.

**Parameters:**
- `client` - EventodbEx client
- `event_type` - Event type name
- `data` - Event payload data
- `event_module` - Event module (must have `stream_name/1`)
- `opts` - Additional publish options (optional)

**Returns:**
- `{:ok, result, client}` - Published successfully
- `{:error, changeset}` - Validation failed
- `{:error, reason}` - Publishing failed

**Example:**
```elixir
EventodbKit.EventDispatcher.publish_event(
  client,
  "UserCreated",
  %{user_id: "123", email: "user@example.com"},
  MyApp.Events.UserCreated,
  %{expected_version: 0}
)
```

## Consumer Patterns

### Basic Consumer

```elixir
defmodule MyApp.BasicConsumer do
  use EventodbKit.Consumer
  
  def handle_message(message, state) do
    case MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, _} -> :ok
      {:error, _} -> :ok  # Log and continue
    end
  end
  
  defp handle_event(MyApp.Events.UserCreated, event) do
    Logger.info("User created: #{event.user_id}")
  end
  
  defp handle_event(_module, _event), do: :ok
end
```

### Error Handling Consumer

```elixir
defmodule MyApp.ValidatingConsumer do
  use EventodbKit.Consumer
  require Logger
  
  def handle_message(message, state) do
    case MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, result} ->
        Logger.debug("Processed #{message["type"]}: #{inspect(result)}")
        :ok
      
      {:error, :unknown_event} ->
        Logger.warn("Unknown event type: #{message["type"]}")
        :ok  # Continue processing
      
      {:error, %Ecto.Changeset{} = changeset} ->
        Logger.error("Validation failed: #{inspect(changeset.errors)}")
        {:error, :validation_failed}  # Stop and retry
    end
  end
  
  defp handle_event(module, event) do
    # Your business logic here
    :ok
  end
end
```

### Multi-Category Consumer

```elixir
defmodule MyApp.AggregateConsumer do
  use EventodbKit.Consumer
  
  def handle_message(message, state) do
    MyApp.Events.dispatch(message["type"], message["data"], fn module, event ->
      # Different handling based on event module
      case module do
        MyApp.Events.UserCreated ->
          create_user_record(event, state)
        
        MyApp.Events.OrderPlaced ->
          process_order(event, state)
        
        MyApp.Events.PaymentProcessed ->
          update_payment_status(event, state)
        
        _ ->
          :ok
      end
    end)
  end
end
```

## Transactional Publishing

Use the `Outbox` module for transactional event publishing:

```elixir
# In your business logic
EventodbKit.TestRepo.transaction(fn ->
  # 1. Perform business operations
  user = create_user_in_db(params)
  
  # 2. Validate event data
  {:ok, validated} = MyApp.Events.validate("UserCreated", %{
    user_id: user.id,
    email: user.email,
    created_at: DateTime.utc_now()
  })
  
  # 3. Store in outbox (atomic with business operation)
  event_module = MyApp.Events.UserCreated
  EventodbKit.Outbox.insert(
    EventodbKit.TestRepo,
    namespace,
    event_module.stream_name(validated),
    "UserCreated",
    Map.from_struct(validated)
  )
end)
```

The `OutboxSender` will automatically publish events from the outbox.

## Generated Event Schema API

Each generated event module provides:

### `changeset(data)`

Creates an Ecto changeset for validation.

```elixir
Events.UserCreated.changeset(%{
  user_id: "123",
  email: "user@example.com"
})
```

### `validate!(data)`

Validates data and returns result.

```elixir
Events.UserCreated.validate!(%{
  user_id: "123",
  email: "user@example.com"
})
# => {:ok, %Events.UserCreated{...}}

Events.UserCreated.validate!(%{invalid: "data"})
# => {:error, %Ecto.Changeset{...}}
```

### `category()`

Returns the event category.

```elixir
Events.UserCreated.category()
# => "user"
```

### `stream_name(data)`

Builds stream name from event data.

```elixir
Events.UserCreated.stream_name(%{user_id: "123"})
# => "user-123"
```

## Testing

### Unit Testing Event Handlers

```elixir
test "handles UserCreated event" do
  data = %{
    "user_id" => "test-123",
    "email" => "test@example.com",
    "created_at" => DateTime.utc_now()
  }
  
  result = MyApp.Events.dispatch("UserCreated", data, fn module, event ->
    assert module == MyApp.Events.UserCreated
    assert event.user_id == "test-123"
    :processed
  end)
  
  assert result == {:ok, :processed}
end
```

### Integration Testing

```elixir
test "consumes events from EventoDB", %{kit: kit} do
  # Write event to EventoDB
  {:ok, _, _} = EventodbEx.stream_write(
    kit.eventodb_client,
    "user-123",
    %{
      type: "UserCreated",
      data: %{
        user_id: "123",
        email: "user@example.com",
        created_at: DateTime.utc_now()
      }
    }
  )
  
  # Consumer will process automatically
  assert_receive {:processed, result}, 1000
end
```

## Best Practices

### 1. Register All Events at Application Start

```elixir
# lib/my_app/events.ex
defmodule MyApp.Events do
  use EventodbKit.EventDispatcher
  
  # User events
  register "UserCreated", MyApp.Events.UserCreated
  register "UserUpdated", MyApp.Events.UserUpdated
  register "UserDeleted", MyApp.Events.UserDeleted
  
  # Order events
  register "OrderPlaced", MyApp.Events.OrderPlaced
  register "OrderShipped", MyApp.Events.OrderShipped
  register "OrderDelivered", MyApp.Events.OrderDelivered
end
```

### 2. Pattern Match on Event Modules

```elixir
# Good: Type-safe pattern matching
defp handle_event(MyApp.Events.UserCreated, event) do
  # event.user_id is known to exist
  create_profile(event.user_id)
end

# Bad: String matching loses type safety
defp handle_event(event_type, data) when event_type == "UserCreated" do
  create_profile(data["user_id"])  # Might be nil!
end
```

### 3. Handle Unknown Events Gracefully

```elixir
case MyApp.Events.dispatch(type, data, &handle_event/2) do
  {:ok, _} -> :ok
  {:error, :unknown_event} -> 
    Logger.debug("Skipping unknown event: #{type}")
    :ok  # Don't block the consumer
  {:error, changeset} ->
    Logger.error("Validation failed: #{inspect(changeset.errors)}")
    {:error, :validation_failed}
end
```

### 4. Use Separate Registries for Different Contexts

```elixir
# User context events
defmodule MyApp.UserEvents do
  use EventodbKit.EventDispatcher
  register "UserCreated", MyApp.Events.UserCreated
  register "UserUpdated", MyApp.Events.UserUpdated
end

# Order context events
defmodule MyApp.OrderEvents do
  use EventodbKit.EventDispatcher
  register "OrderPlaced", MyApp.Events.OrderPlaced
  register "OrderShipped", MyApp.Events.OrderShipped
end
```

### 5. Validate Before Publishing

```elixir
# Always validate event data before publishing
def create_user(params) do
  with {:ok, validated} <- MyApp.Events.validate("UserCreated", params),
       {:ok, _, _} <- publish_event(validated) do
    {:ok, validated}
  end
end
```

## Migration Guide

### From Manual Event Handling

**Before:**
```elixir
def handle_message(message, state) do
  case message["type"] do
    "UserCreated" ->
      data = message["data"]
      create_user(data["user_id"], data["email"])
    
    "OrderPlaced" ->
      data = message["data"]
      process_order(data)
    
    _ -> :ok
  end
end
```

**After:**
```elixir
def handle_message(message, state) do
  MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2)
end

defp handle_event(MyApp.Events.UserCreated, event) do
  # event is validated struct
  create_user(event.user_id, event.email)
end

defp handle_event(MyApp.Events.OrderPlaced, event) do
  process_order(event)
end
```

## Troubleshooting

### Validation Errors

If you get validation errors, check:

1. All required fields are present
2. Field types match schema (e.g., UUIDs, datetimes)
3. Enum values are valid
4. Nested fields (embeds) are properly structured

### Unknown Events

If events are not dispatched:

1. Ensure event is registered in your dispatcher module
2. Check event type string matches exactly
3. Verify generated module is compiled and available

### Performance

For high-throughput scenarios:

1. Use batch processing in consumers
2. Consider parallel consumer groups
3. Monitor validation overhead (usually negligible)
