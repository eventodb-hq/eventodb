# ADR-008: SubscriptionHub for Local Poke Distribution

**Date:** 2025-01-24  
**Status:** Proposed  
**Context:** Reducing polling overhead in EventodbKit consumers

---

## Problem

Currently, each `EventodbKit.Consumer` independently polls EventoDB:

```
Process (e.g., web app instance)
├── OrderConsumer          → polls category_get("order") every 1s
├── PaymentConsumer        → polls category_get("payment") every 1s
├── InventoryConsumer      → polls category_get("inventory") every 1s
└── NotificationConsumer   → polls category_get("notification") every 1s
```

**Scale problem:**
- 4 consumers × 1 poll/sec = 4 requests/sec per process
- 4 processes per service = 16 requests/sec per service
- 10 services = **160 requests/sec** to EventoDB
- All this happens even when **nothing has changed**

This creates:
1. Unnecessary load on EventoDB
2. Network noise masking real traffic
3. Difficulty spotting issues in logs/metrics

---

## Decision

Introduce `EventodbKit.SubscriptionHub` - a single GenServer per process that:

1. Opens **one** SSE connection using `?all=true` (see ADR-007)
2. Receives pokes for all events in the namespace
3. Distributes pokes locally to registered consumers
4. Consumers wait for pokes instead of blind polling

---

## Design

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Process                                  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   SubscriptionHub (gen_statem)            │   │
│  │  States: :connecting → :connected ⇄ :disconnected         │   │
│  │  - SSE: /subscribe?all=true&token=...                     │   │
│  │  - Registry: %{"order" => [pid1], "payment" => [pid2]}    │   │
│  │  - Fallback polling when SSE down                         │   │
│  └──────────────────────────────────────────────────────────┘   │
│         │                    │                    │              │
│         │ poke               │ poke               │ (ignored)    │
│         ▼                    ▼                    ▼              │
│  ┌────────────┐      ┌────────────┐      ┌─────────────┐        │
│  │OrderConsumer│     │PaymentConsumer│   │(no consumer)│        │
│  │ fetch→process│    │ fetch→process │   │             │        │
│  └────────────┘      └────────────┘      └─────────────┘        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ SSE + category_get (on poke only)
                              ▼
                        ┌──────────┐
                        │ EventoDB │
                        └──────────┘
```

### State Machine

```
                    ┌─────────────┐
                    │ :connecting │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │ success    │            │ failure
              ▼            │            ▼
      ┌───────────┐        │    ┌──────────────┐
      │:connected │◄───────┴────│:disconnected │
      └─────┬─────┘  reconnect  └──────┬───────┘
            │                          │
            │ SSE error/timeout        │ fallback poll
            └──────────────────────────┘
```

**State behaviors:**

| State | SSE | Polling | Actions |
|-------|-----|---------|---------|
| `:connecting` | Attempting | None | Try to establish SSE connection |
| `:connected` | Active | Health check only | Distribute pokes, monitor silence |
| `:disconnected` | None | Fallback (5s) | Reconnect with exponential backoff |

### Testable Design: Pure Core + Thin Shell

We separate the state machine into two layers:

1. **Pure Core** (`SubscriptionHub.Core`) - All logic, no side effects
2. **Thin Shell** (`SubscriptionHub`) - gen_statem that executes effects

```
┌─────────────────────────────────────────────────────────────┐
│                    SubscriptionHub (gen_statem)              │
│  - Receives messages (SSE pokes, timers, calls)              │
│  - Calls Core.handle_event(state, data, event)               │
│  - Executes returned effects (send, schedule_timer, etc.)    │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    SubscriptionHub.Core (pure)               │
│  - handle_event(state, data, event) → {state, data, effects}│
│  - No processes, no timers, no IO                            │
│  - 100% deterministic, trivially testable                    │
└─────────────────────────────────────────────────────────────┘
```

**Testing the core is trivial:**
```elixir
test "transitions to :disconnected on SSE error" do
  data = Core.new(kit_fn: fn -> kit end)
  {state, data, _effects} = Core.handle_event(:connected, data, {:sse_error, :timeout})
  
  assert state == :disconnected
  assert data.reconnect_attempts == 1
end

test "exponential backoff increases delay" do
  data = %{Core.new() | reconnect_attempts: 3}
  {_state, _data, effects} = Core.handle_event(:disconnected, data, :reconnect_timeout)
  
  assert {:schedule, :reconnect, delay} = find_effect(effects, :schedule)
  assert delay >= 8_000  # 1000 * 2^3
end
```

### Core Module (Pure)

```elixir
defmodule EventodbKit.SubscriptionHub.Core do
  @moduledoc """
  Pure functional core for SubscriptionHub state machine.
  
  No side effects - returns effects to be executed by the shell.
  """

  defstruct [
    :kit_fn,
    :subscription_ref,
    :last_poke_at,
    registry: %{},
    monitors: %{},
    reconnect_attempts: 0,
    config: %{
      fallback_poll_interval: 5_000,
      health_check_interval: 30_000,
      reconnect_base_delay: 1_000,
      reconnect_max_delay: 30_000
    }
  ]

  @type state :: :connecting | :connected | :disconnected
  @type effect ::
    {:start_subscription, kit_fn :: function()}
    | {:kill_subscription, ref :: any()}
    | {:schedule, name :: atom(), delay_ms :: non_neg_integer()}
    | {:cancel_timer, name :: atom()}
    | {:send, pid :: pid(), message :: any()}
    | {:monitor, pid :: pid()}
    | {:demonitor, ref :: reference()}
    | {:reply, from :: any(), response :: any()}

  @doc "Create new core state"
  def new(opts \\ []) do
    config = %{
      fallback_poll_interval: Keyword.get(opts, :fallback_poll_interval, 5_000),
      health_check_interval: Keyword.get(opts, :health_check_interval, 30_000),
      reconnect_base_delay: Keyword.get(opts, :reconnect_base_delay, 1_000),
      reconnect_max_delay: Keyword.get(opts, :reconnect_max_delay, 30_000)
    }

    %__MODULE__{
      kit_fn: Keyword.get(opts, :kit_fn),
      config: config
    }
  end

  @doc """
  Handle an event and return new state + effects to execute.
  
  Pure function - no side effects.
  """
  @spec handle_event(state(), t(), any()) :: {state(), t(), [effect()]}
  
  # --- State: :connecting ---

  def handle_event(:connecting, data, :enter) do
    effects = [{:start_subscription, data.kit_fn}]
    {:connecting, data, effects}
  end

  def handle_event(:connecting, data, {:subscription_started, ref}) do
    data = %{data | 
      subscription_ref: ref, 
      reconnect_attempts: 0,
      last_poke_at: now()
    }
    {:connected, data, []}
  end

  def handle_event(:connecting, data, {:subscription_failed, _reason}) do
    delay = reconnect_delay(data)
    data = %{data | reconnect_attempts: data.reconnect_attempts + 1}
    effects = [{:schedule, :reconnect, delay}]
    {:disconnected, data, effects}
  end

  def handle_event(:connecting, data, {:register, from, category, pid}) do
    {data, monitor_effect} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effect]
    {:connecting, data, effects}
  end

  def handle_event(:connecting, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:connecting, data, [{:reply, from, :ok}]}
  end

  def handle_event(:connecting, data, {:status, from}) do
    {:connecting, data, [{:reply, from, :connecting}]}
  end

  # --- State: :connected ---

  def handle_event(:connected, data, :enter) do
    effects = [{:schedule, :health_check, data.config.health_check_interval}]
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:sse_poke, poke}) do
    data = %{data | last_poke_at: now()}
    effects = broadcast_effects(data, poke)
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:sse_error, _error}) do
    effects = [
      {:kill_subscription, data.subscription_ref},
      {:cancel_timer, :health_check},
      {:schedule, :reconnect, 0}
    ]
    {:disconnected, data, effects}
  end

  def handle_event(:connected, data, :health_check) do
    silence = now() - data.last_poke_at
    threshold = data.config.health_check_interval * 2

    if silence > threshold do
      effects = [
        {:kill_subscription, data.subscription_ref},
        {:schedule, :reconnect, 0}
      ]
      {:disconnected, data, effects}
    else
      effects = [{:schedule, :health_check, data.config.health_check_interval}]
      {:connected, data, effects}
    end
  end

  def handle_event(:connected, data, {:consumer_down, ref}) do
    data = handle_consumer_down(data, ref)
    {:connected, data, [{:demonitor, ref}]}
  end

  def handle_event(:connected, data, {:register, from, category, pid}) do
    {data, monitor_effect} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effect]
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:connected, data, [{:reply, from, :ok}]}
  end

  def handle_event(:connected, data, {:status, from}) do
    {:connected, data, [{:reply, from, :connected}]}
  end

  # --- State: :disconnected ---

  def handle_event(:disconnected, data, :enter) do
    effects = [{:schedule, :fallback_poll, data.config.fallback_poll_interval}]
    {:disconnected, data, effects}
  end

  def handle_event(:disconnected, data, :reconnect) do
    effects = [
      {:cancel_timer, :fallback_poll},
      {:start_subscription, data.kit_fn}
    ]
    {:connecting, data, effects}
  end

  def handle_event(:disconnected, data, :fallback_poll) do
    poll_effects = fallback_poll_effects(data)
    schedule = [{:schedule, :fallback_poll, data.config.fallback_poll_interval}]
    {:disconnected, data, poll_effects ++ schedule}
  end

  def handle_event(:disconnected, data, {:consumer_down, ref}) do
    data = handle_consumer_down(data, ref)
    {:disconnected, data, [{:demonitor, ref}]}
  end

  def handle_event(:disconnected, data, {:register, from, category, pid}) do
    {data, monitor_effect} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effect]
    {:disconnected, data, effects}
  end

  def handle_event(:disconnected, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:disconnected, data, [{:reply, from, :ok}]}
  end

  def handle_event(:disconnected, data, {:status, from}) do
    {:disconnected, data, [{:reply, from, :disconnected}]}
  end

  # --- Helpers (pure) ---

  defp reconnect_delay(data) do
    base = data.config.reconnect_base_delay
    max = data.config.reconnect_max_delay
    delay = min(base * round(:math.pow(2, data.reconnect_attempts)), max)
    # Deterministic for testing - shell can add jitter
    delay
  end

  defp broadcast_effects(data, poke) do
    category = extract_category(poke.stream)
    
    case Map.get(data.registry, category) do
      nil -> []
      pids -> Enum.map(pids, fn pid -> {:send, pid, {:poke, category, poke}} end)
    end
  end

  defp fallback_poll_effects(data) do
    Enum.flat_map(data.registry, fn {category, pids} ->
      synthetic_poke = %{stream: "#{category}-fallback", global_position: :poll}
      Enum.map(pids, fn pid -> {:send, pid, {:poke, category, synthetic_poke}} end)
    end)
  end

  defp do_register(data, category, pid) do
    registry = Map.update(
      data.registry, 
      category, 
      MapSet.new([pid]), 
      &MapSet.put(&1, pid)
    )
    
    data = %{data | registry: registry}
    {data, [{:monitor, pid}]}
  end

  defp do_unregister(data, category, pid) do
    registry = Map.update(
      data.registry,
      category,
      MapSet.new(),
      &MapSet.delete(&1, pid)
    )
    
    registry = if MapSet.size(Map.get(registry, category, MapSet.new())) == 0 do
      Map.delete(registry, category)
    else
      registry
    end
    
    %{data | registry: registry}
  end

  defp handle_consumer_down(data, ref) do
    case Map.get(data.monitors, ref) do
      nil -> data
      {category, pid} ->
        data = do_unregister(data, category, pid)
        %{data | monitors: Map.delete(data.monitors, ref)}
    end
  end

  defp extract_category(stream_name) do
    case String.split(to_string(stream_name), "-", parts: 2) do
      [category, _id] -> category
      [category] -> category
    end
  end

  # For testing: allow injecting time
  defp now, do: System.monotonic_time(:millisecond)
end
```

### Shell Module (gen_statem wrapper)

The hub uses `gen_statem` for explicit state management with three states:
- `:connecting` - Establishing SSE connection
- `:connected` - SSE active, distributing pokes
- `:disconnected` - SSE failed, fallback to polling

**Resilience features:**
- Automatic reconnection with exponential backoff
- Fallback polling when SSE is down (configurable interval)
- Health check polling to detect silent SSE failures
- Monitors consumer processes for automatic cleanup

```elixir
defmodule EventodbKit.SubscriptionHub do
  @moduledoc """
  Thin gen_statem shell that wraps SubscriptionHub.Core.
  
  All logic lives in Core - this module just:
  1. Translates gen_statem events to Core events
  2. Executes effects returned by Core
  """
  @behaviour :gen_statem
  require Logger

  alias EventodbKit.SubscriptionHub.Core

  ## Public API

  def start_link(opts) do
    name = Keyword.get(opts, :name, __MODULE__)
    :gen_statem.start_link({:local, name}, __MODULE__, opts, [])
  end

  def register(hub \\ __MODULE__, category, consumer_pid) do
    :gen_statem.call(hub, {:register, category, consumer_pid})
  end

  def unregister(hub \\ __MODULE__, category, consumer_pid) do
    :gen_statem.call(hub, {:unregister, category, consumer_pid})
  end

  def status(hub \\ __MODULE__) do
    :gen_statem.call(hub, :status)
  end

  ## gen_statem callbacks

  @impl true
  def callback_mode, do: :handle_event_function

  @impl true
  def init(opts) do
    data = Core.new(opts)
    # Trigger initial :enter event
    {state, data, effects} = Core.handle_event(:connecting, data, :enter)
    execute_effects(effects)
    {:ok, state, data}
  end

  @impl true
  def handle_event(:info, {:sse_poke, poke}, state, data) do
    handle_core_event(state, data, {:sse_poke, poke})
  end

  def handle_event(:info, {:sse_error, error}, state, data) do
    Logger.warning("SubscriptionHub: SSE error: #{inspect(error)}")
    handle_core_event(state, data, {:sse_error, error})
  end

  def handle_event(:info, {:subscription_result, result}, state, data) do
    event = case result do
      {:ok, ref} -> {:subscription_started, ref}
      {:error, reason} -> {:subscription_failed, reason}
    end
    handle_core_event(state, data, event)
  end

  def handle_event(:info, {timer, _ref}, state, data) when timer in [:health_check, :fallback_poll, :reconnect] do
    handle_core_event(state, data, timer)
  end

  def handle_event(:info, {:DOWN, ref, :process, _pid, _reason}, state, data) do
    handle_core_event(state, data, {:consumer_down, ref})
  end

  def handle_event({:call, from}, {:register, category, pid}, state, data) do
    handle_core_event(state, data, {:register, from, category, pid})
  end

  def handle_event({:call, from}, {:unregister, category, pid}, state, data) do
    handle_core_event(state, data, {:unregister, from, category, pid})
  end

  def handle_event({:call, from}, :status, state, data) do
    handle_core_event(state, data, {:status, from})
  end

  ## Private: Core integration

  defp handle_core_event(old_state, data, event) do
    {new_state, new_data, effects} = Core.handle_event(old_state, data, event)
    execute_effects(effects)
    
    if new_state != old_state do
      # State changed - trigger :enter for new state
      {final_state, final_data, enter_effects} = Core.handle_event(new_state, new_data, :enter)
      execute_effects(enter_effects)
      {:next_state, final_state, final_data}
    else
      {:keep_state, new_data}
    end
  end

  ## Private: Effect execution

  defp execute_effects(effects) do
    Enum.each(effects, &execute_effect/1)
  end

  defp execute_effect({:start_subscription, kit_fn}) do
    hub_pid = self()
    # Start subscription async to not block
    Task.start(fn ->
      kit = kit_fn.()
      result = EventodbEx.subscribe_to_all(
        kit.eventodb_client,
        name: "hub-#{System.unique_integer([:positive])}",
        position: 0,
        on_poke: fn poke -> send(hub_pid, {:sse_poke, poke}) end,
        on_error: fn err -> send(hub_pid, {:sse_error, err}) end
      )
      send(hub_pid, {:subscription_result, result})
    end)
  end

  defp execute_effect({:kill_subscription, nil}), do: :ok
  defp execute_effect({:kill_subscription, pid}) when is_pid(pid) do
    if Process.alive?(pid), do: Process.exit(pid, :shutdown)
  end
  defp execute_effect({:kill_subscription, _ref}), do: :ok

  defp execute_effect({:schedule, name, delay}) do
    # Add jitter here (not in Core, to keep Core deterministic)
    jitter = if delay > 0, do: :rand.uniform(max(1, round(delay * 0.1))), else: 0
    Process.send_after(self(), {name, make_ref()}, delay + jitter)
  end

  defp execute_effect({:cancel_timer, _name}) do
    # In practice, we just let old timers fire and ignore them
    # Could track timer refs if we need precise cancellation
    :ok
  end

  defp execute_effect({:send, pid, message}) do
    send(pid, message)
  end

  defp execute_effect({:monitor, pid}) do
    Process.monitor(pid)
  end

  defp execute_effect({:demonitor, ref}) do
    Process.demonitor(ref, [:flush])
  end

  defp execute_effect({:reply, from, response}) do
    :gen_statem.reply(from, response)
  end
end
```

### Consumer Changes

Make `EventodbKit.Consumer` support an optional `:hub` option:

```elixir
defmodule EventodbKit.Consumer do
  # ... existing code ...

  @impl true
  def init({module, opts}) do
    {:ok, consumer_state} = module.init(opts)
    
    # ... existing setup ...
    
    # NEW: Optional hub reference for poke-based mode
    hub = Map.get(consumer_state, :hub)
    
    state = %{
      # ... existing fields ...
      hub: hub
    }

    if hub do
      # Register with hub - wait for pokes instead of polling
      SubscriptionHub.register(hub, state.category, self())
    else
      # Default: schedule traditional polling
      schedule_poll(state)
    end

    {:ok, state}
  end

  @impl true
  def handle_info(:poll, state) do
    # Existing polling behavior (unchanged)
    state = poll_and_process(state)
    schedule_poll(state)
    {:noreply, state}
  end

  # NEW: Handle poke from SubscriptionHub
  def handle_info({:poke, _category, poke}, state) do
    # Only fetch if poke is ahead of our position
    if poke.global_position > (state.position || -1) do
      state = poll_and_process(state)
      {:noreply, state}
    else
      {:noreply, state}
    end
  end

  # ... rest unchanged ...
end
```

### Usage Example

**Before (polling):**
```elixir
defmodule MyApp.ClassConsumer do
  use EventodbKit.Consumer

  def start_link(opts), do: EventodbKit.Consumer.start_link(__MODULE__, opts)

  @impl true
  def init(_opts) do
    {:ok, %{
      category: "class",
      consumer_id: "singleton",
      base_url: "http://localhost:8080",
      token: System.get_env("TOKEN"),
      repo: MyApp.Repo,
      poll_interval: 1_000  # Polls every second
    }}
  end

  @impl true
  def handle_message(message, _state), do: :ok
end
```

**After (poke-based):**
```elixir
defmodule MyApp.ClassConsumer do
  use EventodbKit.Consumer

  def start_link(opts), do: EventodbKit.Consumer.start_link(__MODULE__, opts)

  @impl true
  def init(_opts) do
    kit = MyApp.Kit.get()
    
    {:ok, %{
      category: "class",
      consumer_id: "singleton",
      base_url: kit.eventodb_client.base_url,
      token: kit.eventodb_client.token,
      repo: kit.repo,
      hub: MyApp.SubscriptionHub  # NEW: reference to hub (enables poke mode)
    }}
  end

  @impl true
  def handle_message(message, _state), do: :ok
end

# In application.ex:
children = [
  {EventodbKit.SubscriptionHub, 
    name: MyApp.SubscriptionHub,
    kit_fn: fn -> MyApp.Kit.get() end
  },
  MyApp.ClassConsumer,
  MyApp.MembershipConsumer,
  # ...
]
```

---

## Testing

The pure Core module is trivially testable - no mocking, no async, no timing issues:

```elixir
defmodule EventodbKit.SubscriptionHub.CoreTest do
  use ExUnit.Case, async: true
  
  alias EventodbKit.SubscriptionHub.Core

  describe "connecting state" do
    test "transitions to connected on subscription success" do
      data = Core.new(kit_fn: fn -> :mock_kit end)
      
      {state, data, effects} = Core.handle_event(:connecting, data, {:subscription_started, :sub_ref})
      
      assert state == :connected
      assert data.subscription_ref == :sub_ref
      assert data.reconnect_attempts == 0
    end

    test "transitions to disconnected on subscription failure" do
      data = Core.new(kit_fn: fn -> :mock_kit end)
      
      {state, data, effects} = Core.handle_event(:connecting, data, {:subscription_failed, :econnrefused})
      
      assert state == :disconnected
      assert data.reconnect_attempts == 1
      assert [{:schedule, :reconnect, delay}] = effects
      assert delay == 1_000  # base delay
    end
  end

  describe "connected state" do
    test "broadcasts poke to registered consumers" do
      data = Core.new() |> register_consumer("order", :consumer_pid)
      poke = %{stream: "order-123", global_position: 42}
      
      {state, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})
      
      assert state == :connected
      assert {:send, :consumer_pid, {:poke, "order", ^poke}} in effects
    end

    test "ignores poke for unregistered category" do
      data = Core.new() |> register_consumer("order", :consumer_pid)
      poke = %{stream: "payment-456", global_position: 42}
      
      {state, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})
      
      assert state == :connected
      assert effects == []
    end

    test "transitions to disconnected on SSE error" do
      data = Core.new()
      
      {state, _data, effects} = Core.handle_event(:connected, data, {:sse_error, :timeout})
      
      assert state == :disconnected
      assert {:kill_subscription, _} in effects
      assert {:schedule, :reconnect, 0} in effects
    end
  end

  describe "disconnected state" do
    test "sends fallback poll to all registered consumers" do
      data = Core.new()
        |> register_consumer("order", :pid1)
        |> register_consumer("payment", :pid2)
      
      {state, _data, effects} = Core.handle_event(:disconnected, data, :fallback_poll)
      
      assert state == :disconnected
      assert length(effects) == 3  # 2 sends + 1 schedule
      assert Enum.any?(effects, &match?({:send, :pid1, {:poke, "order", _}}, &1))
      assert Enum.any?(effects, &match?({:send, :pid2, {:poke, "payment", _}}, &1))
    end

    test "transitions to connecting on reconnect" do
      data = Core.new(kit_fn: fn -> :mock_kit end)
      
      {state, _data, effects} = Core.handle_event(:disconnected, data, :reconnect)
      
      assert state == :connecting
      assert {:start_subscription, _} in effects
    end
  end

  describe "exponential backoff" do
    test "increases delay with each attempt" do
      data = Core.new()
      
      # First failure
      {_, data, [{:schedule, :reconnect, delay1}]} = 
        Core.handle_event(:connecting, data, {:subscription_failed, :error})
      assert delay1 == 1_000
      
      # Second failure  
      {_, data, [{:schedule, :reconnect, delay2}]} = 
        Core.handle_event(:connecting, data, {:subscription_failed, :error})
      assert delay2 == 2_000
      
      # Third failure
      {_, data, [{:schedule, :reconnect, delay3}]} = 
        Core.handle_event(:connecting, data, {:subscription_failed, :error})
      assert delay3 == 4_000
    end

    test "caps at max delay" do
      data = %{Core.new() | reconnect_attempts: 10}
      
      {_, _, [{:schedule, :reconnect, delay}]} = 
        Core.handle_event(:connecting, data, {:subscription_failed, :error})
      
      assert delay == 30_000  # max
    end
  end

  # Helper
  defp register_consumer(data, category, pid) do
    {_state, data, _effects} = Core.handle_event(:connecting, data, {:register, :from, category, pid})
    data
  end
end
```

**Key benefits:**
- Tests are synchronous - no `assert_receive`, no timeouts
- No process coordination - just function calls
- Deterministic - same input always produces same output
- Fast - hundreds of tests run in milliseconds
- Effects are data - easy to assert what *would* happen

---

## Resilience Features

### 1. Automatic Reconnection
When SSE connection fails, the hub enters `:disconnected` state and attempts reconnection with exponential backoff:
- Base delay: 1 second
- Max delay: 30 seconds
- Jitter: ±10% to prevent thundering herd

### 2. Fallback Polling
While disconnected, the hub polls at a configurable interval (default: 5s) and sends synthetic pokes to all registered consumers. This ensures consumers don't miss events during outages.

### 3. Health Check
Even when "connected", the hub monitors for silent failures:
- If no poke received in 2× health_check_interval (default: 60s), assume connection is dead
- Triggers reconnection

### 4. Consumer Monitoring
The hub monitors all registered consumer processes:
- Automatic cleanup when consumers crash
- No stale entries in registry

---

## Traffic Comparison

| Scenario | Before (4 consumers) | After |
|----------|---------------------|-------|
| Idle (no events) | 4 HTTP polls/sec | 0 (SSE keepalive only) |
| 1 event in "class" | 4 polls + 1 fetch | 1 poke + 1 fetch |
| 100 events/sec (class) | 4 polls + 100 fetches | 100 pokes + batched fetches |
| Burst in unrelated category | 4 polls | pokes ignored |

**Connection reduction:** N SSE connections → 1 SSE connection per process.

---

## Backward Compatibility

- **Default behavior unchanged:** Without `poll_fn`, consumers poll as before
- **Opt-in:** Services migrate consumers one at a time
- **Graceful fallback:** If hub dies, consumers can fall back to polling

---

## Implementation Steps

1. Add `subscribe_to_all/3` to `EventodbEx` (convenience wrapper)
2. Create `EventodbKit.SubscriptionHub` module
3. Modify `EventodbKit.Consumer` to support `:poll_fn` and `:hub` options
4. Add tests for hub registration/poke distribution
5. Update documentation

---

## Future Enhancements

1. **Batched fetching:** Hub fetches events and distributes them (saves N fetches)
2. **Reconnection logic:** Auto-reconnect SSE with exponential backoff
3. **Metrics:** Track pokes received/ignored per category
4. **Health checks:** Expose hub connection status

---

## Dependencies

- ADR-007: Global SSE Subscription (`?all=true` server support)

---

## References

- [ADR-007: Global SSE Subscription](./ADR007-global-sse-subscription.md)
- [EventodbKit.Consumer](../../clients/eventodb_kit/lib/eventodb_kit/consumer.ex)
