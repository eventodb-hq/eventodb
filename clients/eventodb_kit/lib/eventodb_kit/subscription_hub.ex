defmodule EventodbKit.SubscriptionHub do
  @moduledoc """
  Manages a single SSE subscription to EventoDB and distributes
  pokes to registered local consumers.

  This is a thin gen_statem shell that delegates all logic to
  `EventodbKit.SubscriptionHub.Core`. The shell only:
  1. Translates gen_statem events to Core events
  2. Executes effects returned by Core (send, schedule, monitor, etc.)

  ## States

  - `:connecting` - Attempting to establish SSE connection
  - `:connected` - SSE active, receiving pokes
  - `:disconnected` - SSE failed, using fallback polling

  ## Usage

      # In application.ex
      children = [
        {EventodbKit.SubscriptionHub,
          name: MyApp.SubscriptionHub,
          kit_fn: fn -> MyApp.Kit.get() end}
      ]

  ## Options

  - `:kit_fn` - Required. Function returning EventodbKit.Client
  - `:name` - Process name (default: `EventodbKit.SubscriptionHub`)
  - `:fallback_poll_interval` - Polling interval when SSE down (default: 5000ms)
  - `:health_check_interval` - Interval to verify SSE is alive (default: 30000ms)
  - `:reconnect_base_delay` - Base delay for exponential backoff (default: 1000ms)
  - `:reconnect_max_delay` - Max reconnection delay (default: 30000ms)

  ## Consumer Registration

  Consumers register to receive pokes for specific categories:

      SubscriptionHub.register(MyApp.SubscriptionHub, "order", self())

  When a poke arrives for stream "order-123", all registered consumers
  for category "order" receive `{:poke, "order", poke}` message.
  """
  @behaviour :gen_statem
  require Logger

  alias EventodbKit.SubscriptionHub.Core

  # Track timer refs for cancellation
  defstruct [:core_data, timers: %{}, monitors_to_refs: %{}]

  ## Public API

  @doc """
  Starts the SubscriptionHub process.
  """
  def start_link(opts) do
    name = Keyword.get(opts, :name, __MODULE__)
    :gen_statem.start_link({:local, name}, __MODULE__, opts, [])
  end

  @doc """
  Registers a consumer to receive pokes for a category.

  The consumer process will receive `{:poke, category, poke}` messages.
  """
  def register(hub \\ __MODULE__, category, consumer_pid) do
    :gen_statem.call(hub, {:register, category, consumer_pid})
  end

  @doc """
  Unregisters a consumer from a category.
  """
  def unregister(hub \\ __MODULE__, category, consumer_pid) do
    :gen_statem.call(hub, {:unregister, category, consumer_pid})
  end

  @doc """
  Returns the current connection status: `:connecting`, `:connected`, or `:disconnected`.
  """
  def status(hub \\ __MODULE__) do
    :gen_statem.call(hub, :status)
  end

  ## gen_statem callbacks

  @impl true
  def callback_mode, do: :handle_event_function

  @impl true
  def init(opts) do
    core_data = Core.new(opts)
    shell_data = %__MODULE__{core_data: core_data}

    # Trigger initial :enter event
    {_state, new_core_data, effects} = Core.handle_event(:connecting, core_data, :enter)
    shell_data = %{shell_data | core_data: new_core_data}
    shell_data = execute_effects(shell_data, effects)

    {:ok, :connecting, shell_data}
  end

  @impl true
  # SSE messages
  def handle_event(:info, {:sse_poke, poke}, state, shell_data) do
    handle_core_event(state, shell_data, {:sse_poke, poke})
  end

  def handle_event(:info, {:sse_error, error}, state, shell_data) do
    handle_core_event(state, shell_data, {:sse_error, error})
  end

  # Subscription result (async from Task)
  def handle_event(:info, {:subscription_result, result}, state, shell_data) do
    event =
      case result do
        {:ok, pid} -> {:subscription_started, pid}
        {:error, {:already_started, pid}} -> {:subscription_started, pid}
        {:error, reason} -> {:subscription_failed, reason}
      end

    handle_core_event(state, shell_data, event)
  end

  # Timers
  def handle_event(:info, {:timer, name, timer_ref}, state, shell_data) do
    # Only process if this is the current timer for this name
    if Map.get(shell_data.timers, name) == timer_ref do
      handle_core_event(state, shell_data, name)
    else
      # Stale timer, ignore
      {:keep_state, shell_data}
    end
  end

  # Monitor DOWN
  def handle_event(:info, {:DOWN, ref, :process, _pid, _reason}, state, shell_data) do
    handle_core_event(state, shell_data, {:consumer_down, ref})
  end

  # Calls
  def handle_event({:call, from}, {:register, category, pid}, state, shell_data) do
    handle_core_event(state, shell_data, {:register, from, category, pid})
  end

  def handle_event({:call, from}, {:unregister, category, pid}, state, shell_data) do
    handle_core_event(state, shell_data, {:unregister, from, category, pid})
  end

  def handle_event({:call, from}, :status, state, shell_data) do
    handle_core_event(state, shell_data, {:status, from})
  end

  ## Private: Core integration

  defp handle_core_event(old_state, shell_data, event) do
    {new_state, new_core_data, effects} = Core.handle_event(old_state, shell_data.core_data, event)
    shell_data = %{shell_data | core_data: new_core_data}
    shell_data = execute_effects(shell_data, effects)

    if new_state != old_state do
      # State changed - trigger :enter for new state
      {final_state, final_core_data, enter_effects} =
        Core.handle_event(new_state, shell_data.core_data, :enter)

      shell_data = %{shell_data | core_data: final_core_data}
      shell_data = execute_effects(shell_data, enter_effects)
      {:next_state, final_state, shell_data}
    else
      {:keep_state, shell_data}
    end
  end

  ## Private: Effect execution

  defp execute_effects(shell_data, effects) do
    Enum.reduce(effects, shell_data, &execute_effect/2)
  end

  defp execute_effect({:start_subscription, kit_fn}, shell_data) do
    hub_pid = self()

    # Trap exits temporarily so failed subscription doesn't crash us
    old_trap = Process.flag(:trap_exit, true)

    result =
      try do
        kit = kit_fn.()

        EventodbEx.subscribe_to_all(
          kit.eventodb_client,
          name: "hub-#{System.unique_integer([:positive])}",
          position: 0,
          on_poke: fn poke -> send(hub_pid, {:sse_poke, poke}) end,
          on_error: fn err -> send(hub_pid, {:sse_error, err}) end
        )
      catch
        :exit, reason -> {:error, reason}
        kind, reason -> {:error, {kind, reason}}
      end

    Process.flag(:trap_exit, old_trap)

    # Send to self immediately - GenStateMachine will process it
    send(hub_pid, {:subscription_result, result})

    shell_data
  end

  defp execute_effect({:kill_subscription, nil}, shell_data), do: shell_data

  defp execute_effect({:kill_subscription, pid}, shell_data) when is_pid(pid) do
    if Process.alive?(pid), do: Process.exit(pid, :shutdown)
    shell_data
  end

  defp execute_effect({:kill_subscription, _ref}, shell_data), do: shell_data

  defp execute_effect({:schedule, name, delay}, shell_data) do
    # Add jitter (not in Core to keep it deterministic)
    jitter = if delay > 0, do: :rand.uniform(max(1, round(delay * 0.1))), else: 0
    timer_ref = make_ref()
    Process.send_after(self(), {:timer, name, timer_ref}, delay + jitter)
    %{shell_data | timers: Map.put(shell_data.timers, name, timer_ref)}
  end

  defp execute_effect({:cancel_timer, name}, shell_data) do
    # Just remove from map - old timers will be ignored when they fire
    %{shell_data | timers: Map.delete(shell_data.timers, name)}
  end

  defp execute_effect({:send, pid, message}, shell_data) do
    send(pid, message)
    shell_data
  end

  defp execute_effect({:monitor, pid}, shell_data) do
    ref = Process.monitor(pid)
    # Store in core_data.monitors via updating
    core_data = shell_data.core_data
    # Find the category for this pid from registry
    category = find_category_for_pid(core_data.registry, pid)

    if category do
      monitors = Map.put(core_data.monitors, ref, {category, pid})
      core_data = %{core_data | monitors: monitors}
      %{shell_data | core_data: core_data}
    else
      shell_data
    end
  end

  defp execute_effect({:demonitor, ref}, shell_data) do
    Process.demonitor(ref, [:flush])
    shell_data
  end

  defp execute_effect({:reply, from, response}, shell_data) do
    :gen_statem.reply(from, response)
    shell_data
  end

  defp execute_effect({:log, :info, message}, shell_data) do
    Logger.info(message)
    shell_data
  end

  defp execute_effect({:log, :warning, message}, shell_data) do
    Logger.warning(message)
    shell_data
  end

  defp execute_effect({:log, :error, message}, shell_data) do
    Logger.error(message)
    shell_data
  end

  defp find_category_for_pid(registry, pid) do
    Enum.find_value(registry, fn {category, pids} ->
      if MapSet.member?(pids, pid), do: category
    end)
  end
end
