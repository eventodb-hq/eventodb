# ISSUE008: EventodbKit - Full-Featured Elixir SDK

## Overview

Build `eventodb_kit` - a comprehensive, production-ready Elixir SDK with built-in resilience patterns, sitting on top of the lightweight `eventodb_ex` SDK.

**Package Name:** `eventodb_kit`  
**Depends On:** `eventodb_ex` (path dependency initially, then hex when released)  
**Location:** `/Users/roman/Desktop/work/prj.eventodb/eventodb/clients/eventodb_kit`

## Goals

1. **Outbox Pattern** - Local persistence of writes before sending to EventoDB
2. **Consumer Position Tracking** - Automatic position management per namespace/category/consumer
3. **Idempotency** - Built-in deduplication for producers and consumers
4. **Singleton Workers** - Using `Chosen` for single-instance processing across cluster
5. **String-based Identifiers** - No atoms, dynamic registration via Registry
6. **Test-Driven** - Comprehensive test suite like `eventodb_ex`

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ eventodb_kit (Full SDK)                                 │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ EventodbKit.Client (wraps EventodbEx.Client)    │   │
│  │ + repo, namespace tracking                      │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ EventodbKit.Outbox                              │   │
│  │ - Writes to evento_outbox table                 │   │
│  │ - Background sender (Chosen singleton)          │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ EventodbKit.Consumer                            │   │
│  │ - Position tracking in evento_consumer_positions│   │
│  │ - Deduplication via evento_processed_events     │   │
│  │ - Singleton processing (Chosen)                 │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
└──────────────────┬──────────────────────────────────────┘
                   │ depends on
                   ▼
┌─────────────────────────────────────────────────────────┐
│ eventodb_ex (Lightweight SDK)                           │
│ - HTTP/RPC client                                       │
│ - SSE subscriptions                                     │
│ - No persistence, no background workers                 │
└─────────────────────────────────────────────────────────┘
```

## Database Schemas

EventodbKit provides a `EventodbKit.Migration` module (similar to `Oban.Migration`) that host applications can use to run versioned migrations.

### EventodbKit.Migration Module

```elixir
# lib/eventodb_kit/migration.ex
defmodule EventodbKit.Migration do
  @moduledoc """
  Migrations for EventodbKit tables.
  
  To use, create a migration in your application:
  
      defmodule MyApp.Repo.Migrations.AddEventodbKitTables do
        use Ecto.Migration
        
        def up do
          EventodbKit.Migration.up(version: 1)
        end
        
        def down do
          EventodbKit.Migration.down(version: 1)
        end
      end
  
  Then run: `mix ecto.migrate`
  """
  
  use Ecto.Migration
  
  @initial_version 1
  @current_version 1
  
  def up(opts \\ []) do
    version = Keyword.get(opts, :version, @current_version)
    initial = Keyword.get(opts, :initial, @initial_version)
    
    for v <- initial..version do
      apply_up(v)
    end
  end
  
  def down(opts \\ []) do
    version = Keyword.get(opts, :version, @initial_version)
    current = Keyword.get(opts, :current, @current_version)
    
    for v <- current..version//-1 do
      apply_down(v)
    end
  end
  
  defp apply_up(1) do
    create_if_not_exists table(:evento_outbox, primary_key: false) do
      add :id, :uuid, primary_key: true, default: fragment("gen_random_uuid()")
      add :namespace, :string, null: false
      add :stream, :string, null: false
      add :type, :string, null: false
      add :data, :map, null: false
      add :metadata, :map
      add :write_options, :map
      
      add :sent_at, :utc_datetime_usec
      add :created_at, :utc_datetime_usec, null: false, default: fragment("NOW()")
    end

    create_if_not_exists index(:evento_outbox, [:namespace, :created_at], 
      where: "sent_at IS NULL",
      name: :evento_outbox_unsent_idx
    )
    
    create_if_not_exists index(:evento_outbox, 
      [fragment("(data->>'idempotency_key')")],
      name: :evento_outbox_idempotency_key_idx
    )
    
    create_if_not_exists index(:evento_outbox, [:sent_at])
    
    create_if_not_exists table(:evento_consumer_positions, primary_key: false) do
      add :namespace, :string, null: false, primary_key: true
      add :category, :string, null: false, primary_key: true
      add :consumer_id, :string, null: false, primary_key: true
      add :position, :bigint, null: false, default: 0
      add :updated_at, :utc_datetime
    end

    create_if_not_exists index(:evento_consumer_positions, [:updated_at])
    
    create_if_not_exists table(:evento_processed_events, primary_key: false) do
      add :event_id, :uuid, primary_key: true
      add :namespace, :string, null: false
      add :event_type, :string, null: false
      add :category, :string, null: false
      add :consumer_id, :string, null: false
      add :processed_at, :utc_datetime, null: false, default: fragment("NOW()")
    end

    create_if_not_exists index(:evento_processed_events, [:namespace, :processed_at])
  end
  
  defp apply_down(1) do
    drop_if_exists index(:evento_processed_events, [:namespace, :processed_at])
    drop_if_exists table(:evento_processed_events)
    
    drop_if_exists index(:evento_consumer_positions, [:updated_at])
    drop_if_exists table(:evento_consumer_positions)
    
    drop_if_exists index(:evento_outbox, [:sent_at])
    drop_if_exists index(:evento_outbox, 
      [fragment("(data->>'idempotency_key')")],
      name: :evento_outbox_idempotency_key_idx
    )
    drop_if_exists index(:evento_outbox, [:namespace, :created_at], 
      where: "sent_at IS NULL",
      name: :evento_outbox_unsent_idx
    )
    drop_if_exists table(:evento_outbox)
  end
end
```

### Usage in Host Application

```elixir
# In your application: priv/repo/migrations/20241228_add_eventodb_kit_tables.exs
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

Then run:
```bash
mix ecto.migrate
```

## API Design

### Client Creation (Functional Style)

```elixir
# Create kit client
kit = EventodbKit.Client.new(
  "http://localhost:8080",
  token: "ns_abc123",
  repo: MyApp.Repo
)

# Or wrap existing EventodbEx.Client
eventodb_client = EventodbEx.Client.new("http://localhost:8080", token: "ns_abc123")
kit = EventodbKit.Client.from_eventodb(eventodb_client, repo: MyApp.Repo)

# Create namespace and switch to it
admin_kit = EventodbKit.Client.new("http://localhost:8080", 
  token: admin_token,
  repo: MyApp.Repo
)
{:ok, result, admin_kit} = EventodbKit.namespace_create(admin_kit, "analytics")
analytics_kit = EventodbKit.Client.set_token(admin_kit, result.token)
```

### Write Operations (Outbox Pattern)

```elixir
# Write to outbox (returns immediately)
{:ok, outbox_id, kit} = EventodbKit.stream_write(
  kit,
  "account-123",
  %{type: "Deposited", data: %{amount: 100}},
  %{expected_version: 0}
)

# Write with idempotency key
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

# Batch write
messages = [
  {"stream-1", %{type: "Event1", data: %{}}},
  {"stream-2", %{type: "Event2", data: %{}}},
]
{:ok, outbox_ids, kit} = EventodbKit.stream_write_batch(kit, messages)

# Transactional write (common pattern)
MyApp.Repo.transaction(fn ->
  # Business logic
  lead = MyApp.Repo.insert!(%Lead{email: "test@example.com"})
  
  # Write to outbox in same transaction
  {:ok, _outbox_id, _kit} = EventodbKit.stream_write(
    kit,
    "partnership-#{lead.id}",
    %{type: "LeadCreated", data: %{lead_id: lead.id}}
  )
  
  lead
end)
```

### Read Operations (Delegates to EventodbEx)

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

# Namespace operations
{:ok, result, kit} = EventodbKit.namespace_create(kit, "new-namespace")
{:ok, result, kit} = EventodbKit.namespace_delete(kit, "old-namespace")
{:ok, list, kit} = EventodbKit.namespace_list(kit)
{:ok, info, kit} = EventodbKit.namespace_info(kit, "analytics")
```

### Outbox Sender (Background Worker)

```elixir
defmodule MyApp.Application do
  use Application

  def start(_type, _args) do
    children = [
      MyApp.Repo,
      
      # Chosen lock manager (required)
      {Chosen.LockManager, repo: MyApp.Repo},
      
      # Outbox sender (singleton per namespace)
      {Chosen, 
        child: {EventodbKit.OutboxSender, [
          namespace: "analytics",
          base_url: "http://localhost:8080",
          token: System.get_env("ANALYTICS_TOKEN"),
          repo: MyApp.Repo,
          poll_interval: 1_000,
          batch_size: 100
        ]}, 
        name: :outbox_sender_analytics
      }
    ]

    Supervisor.start_link(children, strategy: :one_for_one)
  end
end
```

### Consumer Pattern (Singleton)

```elixir
defmodule MyApp.PartnershipConsumer do
  use EventodbKit.Consumer
  
  def start_link(opts) do
    EventodbKit.Consumer.start_link(__MODULE__, opts)
  end
  
  @impl EventodbKit.Consumer
  def init(opts) do
    namespace = Keyword.fetch!(opts, :namespace)
    
    {:ok, %{
      namespace: namespace,
      category: "partnership",
      consumer_id: "singleton",
      base_url: "http://localhost:8080",
      token: System.get_env("ANALYTICS_TOKEN"),
      repo: MyApp.Repo,
      poll_interval: 1_000,
      batch_size: 100
    }}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    case message.type do
      "PartnershipApplicationSubmitted" ->
        %MyApp.Lead{
          email: message.data["email"],
          school_name: message.data["school_name"]
        }
        |> state.repo.insert!()
        :ok
        
      _ ->
        Logger.warn("Unknown event type: #{message.type}")
        :ok
    end
  end
end

# In application supervisor (singleton via Chosen)
{Chosen.LockManager, repo: MyApp.Repo},
{Chosen, 
  child: {MyApp.PartnershipConsumer, [namespace: "analytics"]}, 
  name: :partnership_consumer
}
```

### Consumer Group Pattern

```elixir
defmodule MyApp.PartnershipConsumerGroup do
  use EventodbKit.Consumer
  
  def start_link(opts) do
    EventodbKit.Consumer.start_link(__MODULE__, opts)
  end
  
  @impl EventodbKit.Consumer
  def init(opts) do
    namespace = Keyword.fetch!(opts, :namespace)
    group_member = Keyword.fetch!(opts, :group_member)
    group_size = Keyword.fetch!(opts, :group_size)
    
    {:ok, %{
      namespace: namespace,
      category: "partnership",
      consumer_id: "member-#{group_member}",
      group_member: group_member,
      group_size: group_size,
      base_url: "http://localhost:8080",
      token: System.get_env("ANALYTICS_TOKEN"),
      repo: MyApp.Repo,
      poll_interval: 1_000,
      batch_size: 100
    }}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    # Same handler logic
    :ok
  end
end

# In application supervisor (each member as singleton)
children = [
  MyApp.Repo,
  {Chosen.LockManager, repo: MyApp.Repo},
  
  # Member 0
  {Chosen, [
    child: {MyApp.PartnershipConsumerGroup, [
      namespace: "analytics",
      group_member: 0,
      group_size: 3
    ]},
    name: :partnership_consumer_0
  ]},
  
  # Member 1
  {Chosen, [
    child: {MyApp.PartnershipConsumerGroup, [
      namespace: "analytics",
      group_member: 1,
      group_size: 3
    ]},
    name: :partnership_consumer_1
  ]},
  
  # Member 2
  {Chosen, [
    child: {MyApp.PartnershipConsumerGroup, [
      namespace: "analytics",
      group_member: 2,
      group_size: 3
    ]},
    name: :partnership_consumer_2
  ]}
]
```

## Module Structure

```
eventodb_kit/
├── lib/
│   ├── eventodb_kit.ex                    # Main API (delegates to Client)
│   ├── eventodb_kit/
│   │   ├── application.ex                 # Registry, supervisors
│   │   ├── migration.ex                   # Migration module (like Oban.Migration)
│   │   ├── client.ex                      # Core client struct + operations
│   │   ├── schema/
│   │   │   ├── outbox.ex                  # evento_outbox schema
│   │   │   ├── consumer_position.ex       # evento_consumer_positions schema
│   │   │   └── processed_event.ex         # evento_processed_events schema
│   │   ├── outbox.ex                      # Outbox write operations
│   │   ├── outbox_sender.ex               # Background sender worker
│   │   ├── consumer.ex                    # Consumer behaviour + base impl
│   │   ├── consumer/
│   │   │   ├── position.ex                # Position tracking
│   │   │   └── idempotency.ex             # Deduplication
│   │   └── cleanup.ex                     # Cleanup worker (optional)
│   └── mix.exs
└── test/
    ├── test_helper.exs
    ├── eventodb_kit/
    │   ├── migration_test.exs             # Migration tests
    │   ├── outbox_test.exs                # Outbox write tests
    │   ├── outbox_sender_test.exs         # Background sender tests
    │   ├── consumer_test.exs              # Consumer tests
    │   ├── consumer_group_test.exs        # Consumer group tests
    │   ├── idempotency_test.exs           # Deduplication tests
    │   ├── position_test.exs              # Position tracking tests
    │   └── resilience_test.exs            # EventoDB down scenarios
    └── support/
        ├── repo.ex                        # Test repo
        ├── migrations.ex                  # Test migrations
        └── test_consumer.ex               # Test consumer implementations
```

## Dependencies

### mix.exs

```elixir
defmodule EventodbKit.MixProject do
  use Mix.Project

  def project do
    [
      app: :eventodb_kit,
      version: "0.1.0",
      elixir: "~> 1.18",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      aliases: aliases(),
      elixirc_paths: elixirc_paths(Mix.env()),
      description: "Production-ready Elixir SDK for EventoDB with resilience patterns",
      package: package()
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {EventodbKit.Application, []}
    ]
  end

  defp deps do
    [
      # Path dependency initially (until eventodb_ex is released to Hex)
      {:eventodb_ex, path: "../eventodb_ex"},
      
      # Core dependencies
      {:ecto_sql, "~> 3.12"},
      {:postgrex, "~> 0.19"},
      {:jason, "~> 1.4"},
      
      # Singleton workers
      {:chosen, "~> 0.2"},
      
      # Background jobs (optional, for Oban-based sender)
      # {:oban, "~> 2.18", optional: true},
      
      # Dev/test
      {:ex_doc, "~> 0.34", only: :dev, runtime: false}
    ]
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  defp aliases do
    [
      test: ["ecto.create --quiet", "ecto.migrate --quiet", "test"]
    ]
  end

  defp package do
    [
      licenses: ["MIT"],
      links: %{"GitHub" => "https://github.com/yourusername/eventodb-kit"}
    ]
  end
end
```

## Test Strategy

### Test Setup

```elixir
# test/test_helper.exs
ExUnit.start()

# Start test repo
{:ok, _} = EventodbKit.TestRepo.start_link()
Ecto.Adapters.SQL.Sandbox.mode(EventodbKit.TestRepo, :manual)

# Run migrations using EventodbKit.Migration
Ecto.Migrator.run(
  EventodbKit.TestRepo,
  [{1, EventodbKit.Migration}],
  :up,
  all: true,
  log: false
)

defmodule EventodbKit.TestHelper do
  @base_url System.get_env("EVENTODB_URL", "http://localhost:8080")
  @admin_token System.get_env("EVENTODB_ADMIN_TOKEN")

  def create_test_namespace(test_name) do
    # Create namespace via EventodbEx
    admin_client = EventodbEx.Client.new(@base_url, token: @admin_token)
    namespace_id = "test-#{test_name}-#{unique_suffix()}"

    {:ok, result, _} = EventodbEx.namespace_create(admin_client, namespace_id, %{
      description: "Test namespace for #{test_name}"
    })

    # Create kit client
    kit = EventodbKit.Client.new(@base_url,
      token: result.token,
      repo: EventodbKit.TestRepo
    )
    
    {kit, namespace_id, result.token}
  end

  def cleanup_namespace(namespace_id) do
    admin_client = EventodbEx.Client.new(@base_url, token: @admin_token)
    EventodbEx.namespace_delete(admin_client, namespace_id)
  end

  def unique_suffix do
    :erlang.unique_integer([:positive, :monotonic])
    |> Integer.to_string(36)
    |> String.downcase()
  end
end
```

### Test Categories

**1. Outbox Tests** (`test/eventodb_kit/outbox_test.exs`)
- Write to outbox (single message)
- Write with idempotency key (deduplication)
- Batch write
- Transactional write with business logic
- Write options (expected_version, custom id)

**2. Outbox Sender Tests** (`test/eventodb_kit/outbox_sender_test.exs`)
- Send unsent events to EventoDB
- Mark events as sent
- Handle EventoDB unavailable (graceful degradation)
- Batch processing
- Retry logic

**3. Consumer Tests** (`test/eventodb_kit/consumer_test.exs`)
- Start consumer and process messages
- Position tracking (save/load)
- Idempotency (skip already processed)
- Handle unknown event types
- Error handling and retry
- EventoDB unavailable (continue with stale data)

**4. Consumer Group Tests** (`test/eventodb_kit/consumer_group_test.exs`)
- Multiple members process different partitions
- Position tracking per member
- No duplicate processing across members
- All streams covered

**5. Idempotency Tests** (`test/eventodb_kit/idempotency_test.exs`)
- Producer: prevent duplicate events
- Consumer: prevent duplicate processing
- Cleanup old records

**6. Position Tests** (`test/eventodb_kit/position_test.exs`)
- Load position (default 0)
- Save position
- Transactional update
- Multiple consumers different positions

**7. Resilience Tests** (`test/eventodb_kit/resilience_test.exs`)
- EventoDB down: writes still succeed (to outbox)
- EventoDB down: consumer continues gracefully
- EventoDB recovery: outbox drains
- Consumer recovery: resumes from position

## Implementation Order

### Phase 1: Core Foundation
1. ✅ Project setup (`mix new`, dependencies)
2. ✅ EventodbKit.Migration module (like Oban.Migration)
3. ✅ Schemas (Outbox, ConsumerPosition, ProcessedEvent)
4. ✅ EventodbKit.Client (wrap EventodbEx.Client + repo)
5. ✅ Basic write to outbox (no sender yet)
6. ✅ Tests for migrations and outbox writes

### Phase 2: Outbox Sender
7. ✅ OutboxSender worker (poll and send)
8. ✅ Integration with Chosen (singleton)
9. ✅ Tests for sender (including EventoDB down)

### Phase 3: Consumer Base
10. ✅ Consumer behaviour
11. ✅ Position tracking module
12. ✅ Idempotency module (ProcessedEvent)
13. ✅ Basic consumer implementation
14. ✅ Tests for consumer

### Phase 4: Consumer Group
15. ✅ Consumer group support
16. ✅ Tests for consumer group partitioning

### Phase 5: Resilience & Polish
17. ✅ Cleanup workers (optional)
18. ✅ Resilience tests
19. ✅ Documentation
20. ✅ README with examples

## Example Test

```elixir
defmodule EventodbKit.OutboxTest do
  use ExUnit.Case, async: true
  import EventodbKit.TestHelper
  alias EventodbKit.Schema.Outbox

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)
    {kit, namespace_id, _token} = create_test_namespace("outbox")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit}
  end

  test "writes message to outbox", %{kit: kit} do
    stream = "account-123"
    message = %{type: "Deposited", data: %{amount: 100}}

    {:ok, outbox_id, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Verify in database
    outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
    assert outbox.namespace == kit.namespace
    assert outbox.stream == stream
    assert outbox.type == "Deposited"
    assert outbox.data == %{"amount" => 100}
    assert is_nil(outbox.sent_at)
  end

  test "prevents duplicate events with idempotency key", %{kit: kit} do
    stream = "payment-456"
    message = %{
      type: "PaymentRequested",
      data: %{
        amount: 1000,
        idempotency_key: "payment_user123_invoice456"
      }
    }

    # Write once
    {:ok, id1, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Write again with same idempotency key
    {:ok, id2, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Should return existing record
    assert id1 == id2
    
    # Only one record in database
    count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
    assert count == 1
  end

  test "transactional write with business logic", %{kit: kit} do
    EventodbKit.TestRepo.transaction(fn ->
      # Simulate business logic (would be Lead in real app)
      lead_id = Ecto.UUID.generate()

      # Write to outbox in same transaction
      {:ok, _outbox_id, _kit} = EventodbKit.stream_write(
        kit,
        "partnership-#{lead_id}",
        %{type: "LeadCreated", data: %{lead_id: lead_id}}
      )

      # Both succeed or both rollback
      :ok
    end)

    count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
    assert count == 1
  end
end
```

## Success Criteria

- ✅ All tests pass (similar coverage to eventodb_ex)
- ✅ Outbox pattern working (write locally, send in background)
- ✅ Consumer position tracking per namespace
- ✅ Idempotency for producers and consumers
- ✅ Singleton workers via Chosen
- ✅ Graceful handling of EventoDB downtime
- ✅ Clean, documented API
- ✅ No atoms for identifiers (string-based)
- ✅ Works with path dependency on eventodb_ex

## Future Enhancements (Not in Scope)

- Oban-based sender (alternative to GenServer)
- Metrics/telemetry integration
- Automatic cleanup workers
- DynamicSupervisor for multi-namespace management
- String-based registry for ergonomic DX
- Caching layer for reads
- Circuit breaker pattern

## Notes for Developer

1. **Start with tests** - Write failing tests first, then implement
2. **Keep it simple** - Don't over-engineer, follow eventodb_ex patterns
3. **Leverage EventodbEx** - Delegate all HTTP/RPC to underlying client
4. **Focus on resilience** - Everything should handle EventoDB downtime
5. **String identifiers** - Never create atoms from user input
6. **Chosen for singletons** - One sender per namespace, one consumer per category
7. **Transactional writes** - Outbox + business logic in one transaction
8. **Position-based resumption** - Always track position for recovery

## References

- EventodbEx SDK: `/Users/roman/Desktop/work/prj.eventodb/eventodb/clients/eventodb_ex`
- Resilience Patterns: `/Users/roman/Desktop/work/prj.eventodb/eventodb-docs/content/05-resilience.md`
- Consumer Patterns: `/Users/roman/Desktop/work/prj.eventodb/eventodb-docs/content/04-consumer-pattern.md`
- Deduplication: `/Users/roman/Desktop/work/prj.eventodb/eventodb-docs/content/07-deduplication.md`
- Chosen: https://hex.pm/packages/chosen
