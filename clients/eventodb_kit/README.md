# EventodbKit

Production-ready Elixir SDK for EventoDB with built-in resilience patterns.

EventodbKit sits on top of the lightweight [EventodbEx](../eventodb_ex) SDK and provides:

- **Outbox Pattern** - Local persistence of writes before sending to EventoDB
- **Consumer Position Tracking** - Automatic position management per namespace/category/consumer
- **Idempotency** - Built-in deduplication for producers and consumers
- **Background Workers** - GenServer-based outbox sender and consumer
- **Type-Safe Event Handling** - Integration with code-generated event schemas (see [CODEGEN_API.md](CODEGEN_API.md))

## Installation

Add `eventodb_kit` to your list of dependencies in `mix.exs`:

```elixir
def deps do
  [
    {:eventodb_kit, "~> 0.1.0"}
  ]
end
```

## Database Setup

EventodbKit requires database tables for outbox, position tracking, and idempotency.

### Step 1: Add Migration

Create a migration in your application:

```bash
mix ecto.gen.migration add_eventodb_kit_tables
```

### Step 2: Use EventodbKit.Migration

```elixir
defmodule MyApp.Repo.Migrations.AddEventodbKitTables do
  use Ecto.Migration

  def up do
    EventodbKit.Migration.up(version: 1)
  end

  def down do
    EventodbKit.Migration.down(version: 1)
  end
end
```

### Step 3: Run Migration

```bash
mix ecto.migrate
```

## Quick Start

### Type-Safe Events with Code Generation

**For the best developer experience, use with generated event schemas:**

```elixir
# 1. Define your event registry (using generated schemas)
defmodule MyApp.Events do
  use EventodbKit.EventDispatcher
  
  register "UserCreated", MyApp.Events.UserCreated
  register "OrderPlaced", MyApp.Events.OrderPlaced
end

# 2. Use in consumers with automatic validation
defmodule MyApp.MyConsumer do
  use EventodbKit.Consumer
  
  def handle_message(message, state) do
    MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2)
  end
  
  defp handle_event(MyApp.Events.UserCreated, event) do
    # event is validated struct with all fields
    IO.puts("User: #{event.user_id}")
    :ok
  end
end
```

**See [CODEGEN_API.md](CODEGEN_API.md) for complete code generation integration guide.**

## Usage

### Create Client

```elixir
kit = EventodbKit.Client.new(
  "http://localhost:8080",
  token: "ns_abc123",
  repo: MyApp.Repo
)
```

### Write Operations (Outbox Pattern)

Writes go to local database first, then are sent in the background:

```elixir
# Write to outbox
{:ok, outbox_id, kit} = EventodbKit.stream_write(
  kit,
  "account-123",
  %{type: "Deposited", data: %{amount: 100}}
)

# Write with idempotency key (prevents duplicates)
{:ok, outbox_id, kit} = EventodbKit.stream_write(
  kit,
  "payment-456",
  %{
    type: "PaymentRequested",
    data: %{
      amount: 1000,
      idempotency_key: "payment_user123_invoice456"
    }
  }
)

# Transactional write with business logic
MyApp.Repo.transaction(fn ->
  lead = MyApp.Repo.insert!(%Lead{email: "test@example.com"})
  
  {:ok, _outbox_id, _kit} = EventodbKit.stream_write(
    kit,
    "partnership-#{lead.id}",
    %{type: "LeadCreated", data: %{lead_id: lead.id}}
  )
  
  lead
end)
```

### Read Operations

Read operations delegate directly to EventodbEx:

```elixir
# Stream operations
{:ok, messages, kit} = EventodbKit.stream_get(kit, "account-123")
{:ok, message, kit} = EventodbKit.stream_last(kit, "account-123")
{:ok, version, kit} = EventodbKit.stream_version(kit, "account-123")

# Category operations
{:ok, messages, kit} = EventodbKit.category_get(kit, "account", %{
  position: 0,
  batch_size: 100
})
```

### Outbox Sender (Background Worker)

Add to your application supervisor:

```elixir
defmodule MyApp.Application do
  use Application

  def start(_type, _args) do
    children = [
      MyApp.Repo,
      
      # Outbox sender - sends unsent messages in background
      {EventodbKit.OutboxSender, [
        namespace: "analytics",
        base_url: "http://localhost:8080",
        token: System.get_env("ANALYTICS_TOKEN"),
        repo: MyApp.Repo,
        poll_interval: 1_000,  # Poll every second
        batch_size: 100
      ]}
    ]

    Supervisor.start_link(children, strategy: :one_for_one)
  end
end
```

### Consumer Pattern

Create a consumer to process events from a category:

```elixir
defmodule MyApp.PartnershipConsumer do
  use EventodbKit.Consumer
  require Logger
  
  def start_link(opts) do
    EventodbKit.Consumer.start_link(__MODULE__, opts)
  end
  
  @impl EventodbKit.Consumer
  def init(opts) do
    {:ok, %{
      namespace: Keyword.fetch!(opts, :namespace),
      category: "partnership",
      consumer_id: "singleton",
      base_url: "http://localhost:8080",
      token: Keyword.fetch!(opts, :token),
      repo: MyApp.Repo,
      poll_interval: 1_000,
      batch_size: 100
    }}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    case message["type"] do
      "PartnershipApplicationSubmitted" ->
        %MyApp.Lead{
          email: message["data"]["email"],
          school_name: message["data"]["school_name"]
        }
        |> MyApp.Repo.insert!()
        :ok
        
      _ ->
        Logger.warn("Unknown event type: #{message["type"]}")
        :ok
    end
  end
end

# Add to supervisor
{MyApp.PartnershipConsumer, [
  namespace: "analytics",
  token: System.get_env("ANALYTICS_TOKEN")
]}
```

### Consumer Groups

For higher throughput, use consumer groups to partition processing:

```elixir
defmodule MyApp.PartnershipConsumerGroup do
  use EventodbKit.Consumer
  
  @impl EventodbKit.Consumer
  def init(opts) do
    {:ok, %{
      namespace: Keyword.fetch!(opts, :namespace),
      category: "partnership",
      consumer_id: "member-#{Keyword.fetch!(opts, :group_member)}",
      group_member: Keyword.fetch!(opts, :group_member),
      group_size: Keyword.fetch!(opts, :group_size),
      base_url: "http://localhost:8080",
      token: Keyword.fetch!(opts, :token),
      repo: MyApp.Repo,
      poll_interval: 1_000,
      batch_size: 100
    }}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, _state) do
    # Process message
    :ok
  end
end

# In supervisor - start 3 members
children = [
  {MyApp.PartnershipConsumerGroup, [
    namespace: "analytics",
    token: token,
    group_member: 0,
    group_size: 3
  ]},
  {MyApp.PartnershipConsumerGroup, [
    namespace: "analytics",
    token: token,
    group_member: 1,
    group_size: 3
  ]},
  {MyApp.PartnershipConsumerGroup, [
    namespace: "analytics",
    token: token,
    group_member: 2,
    group_size: 3
  ]}
]
```

## Features

### Outbox Pattern

- ✅ Writes succeed even when EventoDB is down
- ✅ Transactional consistency with business logic
- ✅ Automatic retry via background sender
- ✅ Idempotency key support

### Consumer Position Tracking

- ✅ Automatic position save/load per consumer
- ✅ Resume from last position after restart
- ✅ Support for multiple consumers per category

### Idempotency

- ✅ Producer: Prevent duplicate events with idempotency keys
- ✅ Consumer: Prevent duplicate processing via `evento_processed_events`
- ✅ Automatic deduplication

### Resilience

- ✅ EventoDB down: Writes continue (to outbox)
- ✅ EventoDB down: Consumers gracefully handle errors
- ✅ EventoDB recovery: Outbox drains automatically
- ✅ Consumer recovery: Resumes from saved position

## Database Tables

EventodbKit creates three tables:

### evento_outbox

Stores events before they're sent to EventoDB:

- `id` - UUID primary key
- `namespace` - Namespace identifier
- `stream` - Stream name
- `type` - Event type
- `data` - Event data (JSONB)
- `metadata` - Optional metadata (JSONB)
- `write_options` - Write options like expected_version (JSONB)
- `sent_at` - Timestamp when sent (NULL if not sent)
- `created_at` - Timestamp when created

### evento_consumer_positions

Tracks consumer positions:

- `namespace` - Namespace identifier (PK)
- `category` - Category name (PK)
- `consumer_id` - Consumer identifier (PK)
- `position` - Current global position
- `updated_at` - Last update timestamp

### evento_processed_events

Tracks processed events for idempotency:

- `event_id` - Event UUID (PK)
- `namespace` - Namespace identifier
- `event_type` - Event type
- `category` - Category name
- `consumer_id` - Consumer identifier
- `processed_at` - Processing timestamp

## Configuration

### Outbox Sender Options

- `:namespace` - Namespace to process (required)
- `:base_url` - EventoDB URL (required)
- `:token` - Namespace token (required)
- `:repo` - Ecto repo (required)
- `:poll_interval` - Polling interval in ms (default: 1000)
- `:batch_size` - Batch size (default: 100)

### Consumer Options

- `:namespace` - Namespace to consume from (required)
- `:category` - Category to consume (required)
- `:consumer_id` - Unique consumer ID (required)
- `:base_url` - EventoDB URL (required)
- `:token` - Namespace token (required)
- `:repo` - Ecto repo (required)
- `:poll_interval` - Polling interval in ms (default: 1000)
- `:batch_size` - Batch size (default: 100)
- `:group_member` - Group member index (optional, for consumer groups)
- `:group_size` - Total group size (optional, for consumer groups)

## Testing

```bash
# Run tests
cd clients/eventodb_kit
mix test

# Or use the test runner script
bin/run_elixir_kit_specs.sh
```

## License

MIT
