# Complete Example: Order Processing System

This example demonstrates a complete order processing system using EventodbKit with code-generated event schemas.

## Event Schema Definition

First, define your events in JSON:

```json
{
  "events": [
    {
      "name": "OrderPlaced",
      "description": "Customer places an order",
      "category": "order",
      "streamPattern": "order-{order_id}",
      "fields": [
        {"name": "order_id", "type": "uuid", "required": true},
        {"name": "customer_id", "type": "string", "required": true},
        {"name": "items", "type": "array", "required": true},
        {"name": "total", "type": "integer", "required": true},
        {"name": "placed_at", "type": "datetime", "required": true}
      ]
    },
    {
      "name": "OrderPaid",
      "description": "Payment processed for order",
      "category": "order",
      "streamPattern": "order-{order_id}",
      "fields": [
        {"name": "order_id", "type": "uuid", "required": true},
        {"name": "payment_id", "type": "string", "required": true},
        {"name": "amount", "type": "integer", "required": true},
        {"name": "paid_at", "type": "datetime", "required": true}
      ]
    },
    {
      "name": "OrderShipped",
      "description": "Order shipped to customer",
      "category": "order",
      "streamPattern": "order-{order_id}",
      "fields": [
        {"name": "order_id", "type": "uuid", "required": true},
        {"name": "tracking_number", "type": "string", "required": true},
        {"name": "carrier", "type": "string", "required": true},
        {"name": "shipped_at", "type": "datetime", "required": true}
      ]
    }
  ]
}
```

## Generate Event Schemas

```bash
cd eventodb-docs/code-gen
bun run index.ts elixir events.json lib/my_shop/events/
```

This generates:
- `lib/my_shop/events/order_placed.ex`
- `lib/my_shop/events/order_paid.ex`
- `lib/my_shop/events/order_shipped.ex`

## Application Structure

```
lib/
├── my_shop/
│   ├── events.ex              # Event dispatcher registry
│   ├── events/                # Generated event schemas
│   │   ├── order_placed.ex
│   │   ├── order_paid.ex
│   │   └── order_shipped.ex
│   ├── orders/
│   │   ├── order_service.ex   # Business logic
│   │   └── order_consumer.ex  # Event consumer
│   ├── fulfillment/
│   │   └── fulfillment_consumer.ex
│   └── repo.ex
```

## Event Registry

```elixir
# lib/my_shop/events.ex
defmodule MyShop.Events do
  use EventodbKit.EventDispatcher
  
  register "OrderPlaced", MyShop.Events.OrderPlaced
  register "OrderPaid", MyShop.Events.OrderPaid
  register "OrderShipped", MyShop.Events.OrderShipped
end
```

## Order Service (Producer)

```elixir
# lib/my_shop/orders/order_service.ex
defmodule MyShop.Orders.OrderService do
  alias MyShop.{Repo, Events}
  alias MyShop.Orders.Order
  
  @doc """
  Places a new order with transactional event publishing.
  """
  def place_order(customer_id, items) do
    Repo.transaction(fn ->
      # 1. Calculate total
      total = calculate_total(items)
      
      # 2. Create order in database
      order = Repo.insert!(%Order{
        customer_id: customer_id,
        items: items,
        total: total,
        status: "pending"
      })
      
      # 3. Validate event data
      event_data = %{
        order_id: order.id,
        customer_id: customer_id,
        items: items,
        total: total,
        placed_at: DateTime.utc_now()
      }
      
      {:ok, validated} = Events.validate("OrderPlaced", event_data)
      
      # 4. Store event in outbox (atomic with order creation)
      EventodbKit.Outbox.insert(
        Repo,
        get_namespace(),
        Events.OrderPlaced.stream_name(validated),
        "OrderPlaced",
        Map.from_struct(validated)
      )
      
      order
    end)
  end
  
  @doc """
  Records payment for an order.
  """
  def record_payment(order_id, payment_id, amount) do
    Repo.transaction(fn ->
      order = Repo.get!(Order, order_id)
      
      # Update order status
      order = Repo.update!(Ecto.Changeset.change(order, status: "paid"))
      
      # Publish event
      event_data = %{
        order_id: order.id,
        payment_id: payment_id,
        amount: amount,
        paid_at: DateTime.utc_now()
      }
      
      {:ok, validated} = Events.validate("OrderPaid", event_data)
      
      EventodbKit.Outbox.insert(
        Repo,
        get_namespace(),
        Events.OrderPaid.stream_name(validated),
        "OrderPaid",
        Map.from_struct(validated)
      )
      
      order
    end)
  end
  
  defp calculate_total(items) do
    Enum.reduce(items, 0, fn item, acc -> 
      acc + (item["price"] * item["quantity"]) 
    end)
  end
  
  defp get_namespace do
    Application.get_env(:my_shop, :eventodb_namespace)
  end
end
```

## Order Consumer

```elixir
# lib/my_shop/orders/order_consumer.ex
defmodule MyShop.Orders.OrderConsumer do
  use EventodbKit.Consumer
  require Logger
  alias MyShop.{Events, Repo}
  
  def start_link(opts) do
    EventodbKit.Consumer.start_link(__MODULE__, opts)
  end
  
  @impl EventodbKit.Consumer
  def init(opts) do
    # Consumer state can hold any data you need
    {:ok, Map.put(opts, :processed_count, 0)}
  end
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    case Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, :processed} ->
        Logger.info("Processed #{message["type"]} for stream #{message["stream_name"]}")
        
        # Update state
        new_state = Map.update(state, :processed_count, 1, &(&1 + 1))
        {:ok, new_state}
      
      {:error, :unknown_event} ->
        Logger.debug("Skipping unknown event: #{message["type"]}")
        :ok
      
      {:error, changeset} ->
        Logger.error("Validation failed for #{message["type"]}: #{inspect(changeset.errors)}")
        {:error, :validation_failed}
    end
  end
  
  # Event Handlers
  
  defp handle_event(Events.OrderPlaced, event) do
    Logger.info("New order placed: #{event.order_id} for customer #{event.customer_id}")
    Logger.info("Items: #{length(event.items)}, Total: $#{event.total / 100}")
    
    # Send confirmation email
    send_order_confirmation(event.customer_id, event.order_id)
    
    :processed
  end
  
  defp handle_event(Events.OrderPaid, event) do
    Logger.info("Payment received for order #{event.order_id}: $#{event.amount / 100}")
    
    # Notify fulfillment team
    notify_fulfillment(event.order_id)
    
    :processed
  end
  
  defp handle_event(Events.OrderShipped, event) do
    Logger.info("Order #{event.order_id} shipped via #{event.carrier}")
    Logger.info("Tracking: #{event.tracking_number}")
    
    # Send shipping notification
    send_tracking_email(event.order_id, event.tracking_number, event.carrier)
    
    :processed
  end
  
  # Helper functions (implement as needed)
  defp send_order_confirmation(_customer_id, _order_id), do: :ok
  defp notify_fulfillment(_order_id), do: :ok
  defp send_tracking_email(_order_id, _tracking, _carrier), do: :ok
end
```

## Fulfillment Consumer

```elixir
# lib/my_shop/fulfillment/fulfillment_consumer.ex
defmodule MyShop.Fulfillment.FulfillmentConsumer do
  use EventodbKit.Consumer
  require Logger
  alias MyShop.Events
  
  @impl EventodbKit.Consumer
  def handle_message(message, state) do
    case Events.dispatch(message["type"], message["data"], &handle_event/2) do
      {:ok, _} -> :ok
      {:error, :unknown_event} -> :ok
      {:error, _} -> {:error, :validation_failed}
    end
  end
  
  # Only handle OrderPaid for fulfillment
  defp handle_event(Events.OrderPaid, event) do
    Logger.info("Processing fulfillment for order #{event.order_id}")
    
    # Create pick list
    create_pick_list(event.order_id)
    
    # Allocate inventory
    allocate_inventory(event.order_id)
    
    :ok
  end
  
  defp handle_event(_module, _event), do: :ok
  
  defp create_pick_list(_order_id), do: :ok
  defp allocate_inventory(_order_id), do: :ok
end
```

## Application Supervisor

```elixir
# lib/my_shop/application.ex
defmodule MyShop.Application do
  use Application
  
  def start(_type, _args) do
    children = [
      # Database
      MyShop.Repo,
      
      # Outbox sender (publishes to EventoDB)
      {EventodbKit.OutboxSender, [
        repo: MyShop.Repo,
        namespace: get_namespace(),
        base_url: get_eventodb_url(),
        token: get_eventodb_token(),
        poll_interval: 1000,
        batch_size: 100
      ]},
      
      # Order consumer (reads all order events)
      {MyShop.Orders.OrderConsumer, [
        namespace: get_namespace(),
        category: "order",
        consumer_id: "order-consumer",
        base_url: get_eventodb_url(),
        token: get_eventodb_token(),
        repo: MyShop.Repo,
        poll_interval: 100,
        batch_size: 10
      ]},
      
      # Fulfillment consumer (only reads order events for fulfillment)
      {MyShop.Fulfillment.FulfillmentConsumer, [
        namespace: get_namespace(),
        category: "order",
        consumer_id: "fulfillment-consumer",
        base_url: get_eventodb_url(),
        token: get_eventodb_token(),
        repo: MyShop.Repo,
        poll_interval: 100,
        batch_size: 10
      ]}
    ]
    
    opts = [strategy: :one_for_one, name: MyShop.Supervisor]
    Supervisor.start_link(children, opts)
  end
  
  defp get_namespace, do: Application.get_env(:my_shop, :eventodb_namespace)
  defp get_eventodb_url, do: Application.get_env(:my_shop, :eventodb_url)
  defp get_eventodb_token, do: Application.get_env(:my_shop, :eventodb_token)
end
```

## Configuration

```elixir
# config/config.exs
config :my_shop,
  ecto_repos: [MyShop.Repo],
  eventodb_url: "http://localhost:8080",
  eventodb_namespace: "my-shop-prod",
  eventodb_token: System.get_env("EVENTODB_TOKEN")

# config/dev.exs
config :my_shop,
  eventodb_url: "http://localhost:8080",
  eventodb_namespace: "my-shop-dev"
```

## Usage Example

```elixir
# Place an order
{:ok, order} = MyShop.Orders.OrderService.place_order(
  "customer-123",
  [
    %{"sku" => "WIDGET-1", "quantity" => 2, "price" => 2999},
    %{"sku" => "GADGET-5", "quantity" => 1, "price" => 4999}
  ]
)

# The OrderPlaced event is stored in outbox
# OutboxSender publishes to EventoDB
# Both consumers receive and process the event

# Record payment
{:ok, order} = MyShop.Orders.OrderService.record_payment(
  order.id,
  "pay_abc123",
  10997
)

# OrderPaid event triggers:
# - OrderConsumer: Sends notification
# - FulfillmentConsumer: Creates pick list
```

## Event Flow

```
OrderService.place_order()
    ↓
Insert Order + Insert Outbox (transaction)
    ↓
OutboxSender polls outbox
    ↓
Publish OrderPlaced to EventoDB
    ↓
OrderConsumer receives event
    ├→ Validates via Events.dispatch()
    ├→ Sends confirmation email
    └→ Updates position
    
FulfillmentConsumer receives event
    ├→ Validates via Events.dispatch()
    ├→ Ignores (only cares about OrderPaid)
    └→ Updates position
```

## Testing

```elixir
defmodule MyShop.Orders.OrderServiceTest do
  use MyShop.DataCase
  alias MyShop.Orders.OrderService
  alias MyShop.{Repo, Events}
  
  test "places order and creates event" do
    items = [
      %{"sku" => "TEST-1", "quantity" => 1, "price" => 1000}
    ]
    
    {:ok, order} = OrderService.place_order("customer-123", items)
    
    # Verify order created
    assert order.customer_id == "customer-123"
    assert order.total == 1000
    
    # Verify event in outbox
    outbox = Repo.one(EventodbKit.Schema.Outbox)
    assert outbox.type == "OrderPlaced"
    assert outbox.stream == "order-#{order.id}"
    
    # Verify event can be validated
    {:ok, event} = Events.validate("OrderPlaced", outbox.data)
    assert event.order_id == order.id
    assert event.customer_id == "customer-123"
  end
end
```

## Benefits

✅ **Type Safety** - Pattern matching on event modules
✅ **Validation** - Automatic validation on all events
✅ **Transactional** - Outbox ensures events are never lost
✅ **Idempotent** - Automatic deduplication
✅ **Scalable** - Multiple consumers can process same events
✅ **Testable** - Easy to test event handling logic
✅ **Maintainable** - Generated schemas reduce boilerplate

## Next Steps

1. Add more event types (OrderCancelled, OrderRefunded, etc.)
2. Implement read models from events
3. Add event versioning
4. Set up monitoring and alerting
5. Configure consumer groups for scaling
