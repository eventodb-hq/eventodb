# Using Code-Generated Events with EventodbKit

EventodbKit works seamlessly with events generated from the EventoDB code generator.

## Quick Example

### 1. Generate Event Schemas

```bash
cd eventodb-docs/code-gen
bun run src/cli.js elixir ../../my_app/lib/events
```

This creates files like `lib/events/partnership_application_submitted.ex`:

```elixir
defmodule Events.PartnershipApplicationSubmitted do
  use Ecto.Schema
  import Ecto.Changeset
  
  @primary_key false
  embedded_schema do
    field :application_id, :binary_id
    field :school_name, :string
    field :contact_name, :string
    field :contact_email, :string
    # ... more fields
  end
  
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:application_id, :school_name, ...])
    |> validate_required([...])
    |> validate_format(:contact_email, ~r/^[^\s@]+@[^\s@]+\.[^\s@]+$/)
  end
  
  def category, do: "partnership_application"
  def stream_name(data), do: "partnership_application-#{data.application_id}"
  def validate!(data), do: # returns {:ok, validated} or {:error, changeset}
end
```

### 2. Write Events with Validation

```elixir
alias Events.PartnershipApplicationSubmitted

# Validate data first
event_data = %{
  application_id: Ecto.UUID.generate(),
  school_name: "Springfield Elementary",
  contact_name: "Seymour Skinner",
  contact_email: "principal@springfield.edu",
  contact_phone: "555-0123",
  submitted_at: DateTime.utc_now()
}

case PartnershipApplicationSubmitted.validate!(event_data) do
  {:ok, validated_event} ->
    # Write to outbox using generated helpers
    stream = PartnershipApplicationSubmitted.stream_name(validated_event)
    category = PartnershipApplicationSubmitted.category()
    
    {:ok, outbox_id, kit} = EventodbKit.stream_write(
      kit,
      stream,
      %{
        type: "PartnershipApplicationSubmitted",
        data: Map.from_struct(validated_event)
      }
    )
    
  {:error, changeset} ->
    # Handle validation errors
    IO.inspect(changeset.errors)
end
```

### 3. Helper Module for Cleaner API

Create a helper to make it even easier:

```elixir
defmodule MyApp.EventPublisher do
  @moduledoc """
  Helper for publishing validated events to EventodbKit.
  """
  
  @doc """
  Publishes a validated event to the outbox.
  
  ## Example
  
      event_data = %{
        application_id: uuid,
        school_name: "Test School",
        ...
      }
      
      EventPublisher.publish(
        kit,
        Events.PartnershipApplicationSubmitted,
        event_data
      )
  """
  def publish(kit, event_module, data) do
    with {:ok, validated} <- event_module.validate!(data) do
      stream = event_module.stream_name(validated)
      event_type = event_module |> Module.split() |> List.last()
      
      EventodbKit.stream_write(
        kit,
        stream,
        %{
          type: event_type,
          data: Map.from_struct(validated)
        }
      )
    end
  end
  
  @doc """
  Publishes with additional options (expected_version, metadata, etc.)
  """
  def publish(kit, event_module, data, opts) do
    with {:ok, validated} <- event_module.validate!(data) do
      stream = event_module.stream_name(validated)
      event_type = event_module |> Module.split() |> List.last()
      
      message = %{
        type: event_type,
        data: Map.from_struct(validated),
        metadata: Keyword.get(opts, :metadata)
      }
      
      write_opts = Keyword.take(opts, [:expected_version, :id])
      
      EventodbKit.stream_write(kit, stream, message, write_opts)
    end
  end
end
```

Now publishing is super clean:

```elixir
alias MyApp.EventPublisher
alias Events.PartnershipApplicationSubmitted

event_data = %{
  application_id: uuid,
  school_name: "Test School",
  contact_name: "John Doe",
  contact_email: "john@test.com",
  contact_phone: "555-0123",
  submitted_at: DateTime.utc_now()
}

# Simple publish
{:ok, outbox_id, kit} = EventPublisher.publish(
  kit,
  PartnershipApplicationSubmitted,
  event_data
)

# With options
{:ok, outbox_id, kit} = EventPublisher.publish(
  kit,
  PartnershipApplicationSubmitted,
  event_data,
  expected_version: 0,
  metadata: %{user_id: "admin-123"}
)
```

### 4. Consumer with Type-Safe Handling

```elixir
defmodule MyApp.PartnershipConsumer do
  use EventodbKit.Consumer
  require Logger
  
  alias Events.PartnershipApplicationSubmitted
  alias Events.PartnershipActivated
  
  @impl EventodbKit.Consumer
  def init(opts) do
    {:ok, %{
      namespace: Keyword.fetch!(opts, :namespace),
      category: "partnership_application",
      consumer_id: "processor",
      base_url: "http://localhost:8080",
      token: Keyword.fetch!(opts, :token),
      repo: MyApp.Repo
    }}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    case message["type"] do
      "PartnershipApplicationSubmitted" ->
        handle_application_submitted(message["data"])
        
      "PartnershipActivated" ->
        handle_partnership_activated(message["data"])
        
      _ ->
        Logger.warn("Unknown event: #{message["type"]}")
        :ok
    end
  end
  
  defp handle_application_submitted(data) do
    # Validate incoming data with generated schema
    case PartnershipApplicationSubmitted.validate!(data) do
      {:ok, event} ->
        # Type-safe processing
        %MyApp.Lead{
          email: event.contact_email,
          school_name: event.school_name,
          contact_name: event.contact_name,
          phone: event.contact_phone
        }
        |> MyApp.Repo.insert!()
        
        :ok
        
      {:error, changeset} ->
        Logger.error("Invalid event data: #{inspect(changeset.errors)}")
        {:error, :invalid_event}
    end
  end
  
  defp handle_partnership_activated(data) do
    case PartnershipActivated.validate!(data) do
      {:ok, event} ->
        # Process activated partnership
        :ok
        
      {:error, changeset} ->
        Logger.error("Invalid event data: #{inspect(changeset.errors)}")
        {:error, :invalid_event}
    end
  end
end
```

## Pattern Matching Helper

For even cleaner consumer code, create a dispatcher:

```elixir
defmodule MyApp.EventDispatcher do
  @moduledoc """
  Type-safe event dispatcher for consumers.
  """
  
  @events %{
    "PartnershipApplicationSubmitted" => Events.PartnershipApplicationSubmitted,
    "PartnershipActivated" => Events.PartnershipActivated,
    "ClassJoinRequested" => Events.ClassJoinRequested
  }
  
  @doc """
  Dispatches event to handler with validation.
  """
  def dispatch(event_type, data, handler) do
    case Map.get(@events, event_type) do
      nil ->
        {:error, :unknown_event}
        
      event_module ->
        with {:ok, validated} <- event_module.validate!(data) do
          handler.(event_module, validated)
        end
    end
  end
end
```

Consumer becomes super clean:

```elixir
defmodule MyApp.PartnershipConsumer do
  use EventodbKit.Consumer
  alias MyApp.EventDispatcher
  alias Events.{PartnershipApplicationSubmitted, PartnershipActivated}
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    EventDispatcher.dispatch(
      message["type"],
      message["data"],
      &handle_event/2
    )
  end
  
  # Pattern match on the event module!
  defp handle_event(PartnershipApplicationSubmitted, event) do
    %MyApp.Lead{
      email: event.contact_email,
      school_name: event.school_name
    }
    |> MyApp.Repo.insert!()
    
    :ok
  end
  
  defp handle_event(PartnershipActivated, event) do
    # Handle activation
    :ok
  end
end
```

## Benefits

✅ **Type Safety** - Compile-time checks for event fields
✅ **Validation** - Automatic validation before publishing
✅ **Documentation** - Generated modules include docs
✅ **DRY** - Stream names and categories centralized
✅ **Refactor-Safe** - IDE can track event structure changes
✅ **Consistent** - Same schemas for producer and consumer

## Full Example: Transactional Publish

```elixir
defmodule MyApp.Partnerships do
  alias MyApp.{Repo, EventPublisher}
  alias Events.{PartnershipApplicationSubmitted, PartnershipActivated}
  
  def submit_application(attrs) do
    application_id = Ecto.UUID.generate()
    
    Repo.transaction(fn ->
      # Business logic
      application = %Application{
        id: application_id,
        school_name: attrs.school_name,
        status: :pending
      }
      |> Repo.insert!()
      
      # Publish event with validation
      event_data = %{
        application_id: application_id,
        school_name: attrs.school_name,
        contact_name: attrs.contact_name,
        contact_email: attrs.contact_email,
        contact_phone: attrs.contact_phone,
        submitted_at: DateTime.utc_now()
      }
      
      case EventPublisher.publish(kit, PartnershipApplicationSubmitted, event_data) do
        {:ok, _outbox_id, _kit} ->
          application
          
        {:error, changeset} ->
          Repo.rollback({:validation_failed, changeset})
      end
    end)
  end
  
  def activate_partnership(application_id) do
    Repo.transaction(fn ->
      application = Repo.get!(Application, application_id)
      application = Ecto.Changeset.change(application, status: :active)
      |> Repo.update!()
      
      event_data = %{
        application_id: application.id,
        activated_at: DateTime.utc_now()
      }
      
      {:ok, _outbox_id, _kit} = EventPublisher.publish(
        kit,
        PartnershipActivated,
        event_data
      )
      
      application
    end)
  end
end
```

## Summary

The code generator creates **Ecto embedded schemas** that:
- Provide validation via `changeset/1`
- Include category and stream name helpers
- Work perfectly with EventodbKit's `stream_write/3`

Combined with EventodbKit's outbox pattern, you get:
- **Type-safe events** from code generator
- **Reliable delivery** from EventodbKit
- **Clean APIs** with helper modules
- **Transactional consistency** with business logic
