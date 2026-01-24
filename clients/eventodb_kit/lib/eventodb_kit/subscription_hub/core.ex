defmodule EventodbKit.SubscriptionHub.Core do
  @moduledoc """
  Pure functional core for SubscriptionHub state machine.

  No side effects - returns effects to be executed by the shell.

  ## States

  - `:connecting` - Attempting to establish SSE connection
  - `:connected` - SSE active, receiving pokes
  - `:disconnected` - SSE failed, using fallback polling

  ## Usage

  The Core is driven by events and returns a tuple of `{new_state, new_data, effects}`.
  The shell (gen_statem) executes the effects and feeds real-world events back.

      data = Core.new(kit_fn: fn -> kit end)
      {state, data, effects} = Core.handle_event(:connecting, data, :enter)
      # Shell executes effects...
      # Shell receives SSE message...
      {state, data, effects} = Core.handle_event(state, data, {:sse_poke, poke})
  """

  @type state :: :connecting | :connected | :disconnected

  @type effect ::
          {:start_subscription, kit_fn :: (-> any())}
          | {:kill_subscription, ref :: any()}
          | {:schedule, name :: atom(), delay_ms :: non_neg_integer()}
          | {:cancel_timer, name :: atom()}
          | {:send, pid :: pid(), message :: any()}
          | {:monitor, pid :: pid()}
          | {:demonitor, ref :: reference()}
          | {:reply, from :: any(), response :: any()}
          | {:log, level :: :info | :warning | :error, message :: String.t()}

  @type t :: %__MODULE__{
          kit_fn: (-> any()) | nil,
          subscription_ref: any(),
          last_poke_at: integer() | nil,
          registry: %{String.t() => MapSet.t(pid())},
          monitors: %{reference() => {String.t(), pid()}},
          reconnect_attempts: non_neg_integer(),
          config: %{
            fallback_poll_interval: non_neg_integer(),
            health_check_interval: non_neg_integer(),
            reconnect_base_delay: non_neg_integer(),
            reconnect_max_delay: non_neg_integer()
          },
          now_fn: (-> integer())
        }

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
      reconnect_max_delay: 30_000,
      quiet: false
    },
    now_fn: &__MODULE__.system_now/0
  ]

  @doc "System monotonic time in milliseconds (default time source)"
  def system_now, do: System.monotonic_time(:millisecond)

  @doc "Create new core state"
  @spec new(keyword()) :: t()
  def new(opts \\ []) do
    config = %{
      fallback_poll_interval: Keyword.get(opts, :fallback_poll_interval, 5_000),
      health_check_interval: Keyword.get(opts, :health_check_interval, 30_000),
      reconnect_base_delay: Keyword.get(opts, :reconnect_base_delay, 1_000),
      reconnect_max_delay: Keyword.get(opts, :reconnect_max_delay, 30_000),
      quiet: Keyword.get(opts, :quiet, false)
    }

    %__MODULE__{
      kit_fn: Keyword.get(opts, :kit_fn),
      config: config,
      now_fn: Keyword.get(opts, :now_fn, &__MODULE__.system_now/0)
    }
  end

  @doc """
  Handle an event and return new state + effects to execute.

  Pure function - no side effects.
  """
  @spec handle_event(state(), t(), any()) :: {state(), t(), [effect()]}

  # ==========================================================================
  # State: :connecting
  # ==========================================================================

  def handle_event(:connecting, data, :enter) do
    attempt_msg =
      if data.reconnect_attempts > 0 do
        " (attempt #{data.reconnect_attempts + 1})"
      else
        ""
      end

    effects =
      [
        {:log, :info, "[SubscriptionHub] Connecting to EventoDB#{attempt_msg}..."},
        {:start_subscription, data.kit_fn}
      ]
      |> maybe_quiet(data)

    {:connecting, data, effects}
  end

  def handle_event(:connecting, data, {:subscription_started, ref}) do
    data = %{data | subscription_ref: ref, reconnect_attempts: 0, last_poke_at: data.now_fn.()}

    effects =
      [{:log, :info, "[SubscriptionHub] ✓ Connected to EventoDB - SSE subscription active"}]
      |> maybe_quiet(data)

    {:connected, data, effects}
  end

  def handle_event(:connecting, data, {:subscription_failed, reason}) do
    delay = reconnect_delay(data)
    data = %{data | reconnect_attempts: data.reconnect_attempts + 1}
    delay_sec = Float.round(delay / 1000, 1)

    effects =
      [
        {:log, :warning, "[SubscriptionHub] Connection failed: #{format_error(reason)} - retrying in #{delay_sec}s"},
        {:schedule, :reconnect, delay}
      ]
      |> maybe_quiet(data)

    {:disconnected, data, effects}
  end

  def handle_event(:connecting, data, {:register, from, category, pid}) do
    {data, monitor_effects} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effects]
    {:connecting, data, effects}
  end

  def handle_event(:connecting, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:connecting, data, [{:reply, from, :ok}]}
  end

  def handle_event(:connecting, data, {:status, from}) do
    {:connecting, data, [{:reply, from, :connecting}]}
  end

  def handle_event(:connecting, data, {:consumer_down, ref}) do
    data = handle_consumer_down(data, ref)
    {:connecting, data, [{:demonitor, ref}]}
  end

  # Ignore SSE errors while connecting (we'll get subscription_failed instead)
  def handle_event(:connecting, data, {:sse_error, _error}) do
    {:connecting, data, []}
  end

  # ==========================================================================
  # State: :connected
  # ==========================================================================

  # Ignore late subscription results (from previous connection attempts)
  def handle_event(:connected, data, {:subscription_started, _ref}) do
    {:connected, data, []}
  end

  def handle_event(:connected, data, {:subscription_failed, _reason}) do
    {:connected, data, []}
  end

  def handle_event(:connected, data, :enter) do
    effects = [{:schedule, :health_check, data.config.health_check_interval}]
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:sse_poke, poke}) do
    data = %{data | last_poke_at: data.now_fn.()}
    effects = broadcast_effects(data, poke)
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:sse_error, error}) do
    effects =
      [
        {:log, :warning, "[SubscriptionHub] SSE connection lost: #{format_error(error)}"},
        {:kill_subscription, data.subscription_ref},
        {:cancel_timer, :health_check},
        {:schedule, :reconnect, 0}
      ]
      |> maybe_quiet(data)

    {:disconnected, data, effects}
  end

  def handle_event(:connected, data, :health_check) do
    now = data.now_fn.()
    silence = now - (data.last_poke_at || now)
    threshold = data.config.health_check_interval * 2

    if silence > threshold do
      silence_sec = Float.round(silence / 1000, 1)

      effects =
        [
          {:log, :warning, "[SubscriptionHub] No activity for #{silence_sec}s - connection may be dead, reconnecting"},
          {:kill_subscription, data.subscription_ref},
          {:schedule, :reconnect, 0}
        ]
        |> maybe_quiet(data)

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
    {data, monitor_effects} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effects]
    {:connected, data, effects}
  end

  def handle_event(:connected, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:connected, data, [{:reply, from, :ok}]}
  end

  def handle_event(:connected, data, {:status, from}) do
    {:connected, data, [{:reply, from, :connected}]}
  end

  # ==========================================================================
  # State: :disconnected
  # ==========================================================================

  # Ignore late subscription results (from previous connection attempts)
  def handle_event(:disconnected, data, {:subscription_started, _ref}) do
    {:disconnected, data, []}
  end

  def handle_event(:disconnected, data, {:subscription_failed, _reason}) do
    {:disconnected, data, []}
  end

  def handle_event(:disconnected, data, :enter) do
    poll_sec = Float.round(data.config.fallback_poll_interval / 1000, 1)

    effects =
      [
        {:log, :info, "[SubscriptionHub] Using fallback polling (every #{poll_sec}s)"},
        {:schedule, :fallback_poll, data.config.fallback_poll_interval}
      ]
      |> maybe_quiet(data)

    {:disconnected, data, effects}
  end

  def handle_event(:disconnected, data, :reconnect) do
    # Just cancel fallback polling and transition to :connecting
    # The :enter handler for :connecting will start the subscription
    effects = [
      {:cancel_timer, :fallback_poll}
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
    {data, monitor_effects} = do_register(data, category, pid)
    effects = [{:reply, from, :ok} | monitor_effects]
    {:disconnected, data, effects}
  end

  def handle_event(:disconnected, data, {:unregister, from, category, pid}) do
    data = do_unregister(data, category, pid)
    {:disconnected, data, [{:reply, from, :ok}]}
  end

  def handle_event(:disconnected, data, {:status, from}) do
    {:disconnected, data, [{:reply, from, :disconnected}]}
  end

  # Ignore SSE errors while disconnected (stale errors from previous connection)
  def handle_event(:disconnected, data, {:sse_error, _error}) do
    {:disconnected, data, []}
  end

  # ==========================================================================
  # Helpers (pure)
  # ==========================================================================

  @doc "Calculate reconnect delay with exponential backoff"
  @spec reconnect_delay(t()) :: non_neg_integer()
  def reconnect_delay(data) do
    base = data.config.reconnect_base_delay
    max = data.config.reconnect_max_delay
    delay = base * round(:math.pow(2, data.reconnect_attempts))
    min(delay, max)
  end

  @doc "Extract category from stream name"
  @spec extract_category(String.t()) :: String.t()
  def extract_category(stream_name) do
    case String.split(to_string(stream_name), "-", parts: 2) do
      [category, _id] -> category
      [category] -> category
    end
  end

  # Private helpers

  defp broadcast_effects(data, poke) do
    category = extract_category(poke.stream)

    case Map.get(data.registry, category) do
      nil -> []
      pids -> Enum.map(MapSet.to_list(pids), fn pid -> {:send, pid, {:poke, category, poke}} end)
    end
  end

  defp fallback_poll_effects(data) do
    Enum.flat_map(data.registry, fn {category, pids} ->
      synthetic_poke = %{stream: "#{category}-fallback", global_position: :poll}

      Enum.map(MapSet.to_list(pids), fn pid ->
        {:send, pid, {:poke, category, synthetic_poke}}
      end)
    end)
  end

  defp do_register(data, category, pid) do
    # Check if already registered
    already_registered? =
      Enum.any?(data.monitors, fn {_ref, {cat, p}} -> cat == category and p == pid end)

    if already_registered? do
      {data, []}
    else
      registry =
        Map.update(
          data.registry,
          category,
          MapSet.new([pid]),
          &MapSet.put(&1, pid)
        )

      data = %{data | registry: registry}
      {data, [{:monitor, pid}]}
    end
  end

  defp do_unregister(data, category, pid) do
    registry =
      Map.update(
        data.registry,
        category,
        MapSet.new(),
        &MapSet.delete(&1, pid)
      )

    # Clean up empty categories
    registry =
      if MapSet.size(Map.get(registry, category, MapSet.new())) == 0 do
        Map.delete(registry, category)
      else
        registry
      end

    %{data | registry: registry}
  end

  defp handle_consumer_down(data, ref) do
    case Map.get(data.monitors, ref) do
      nil ->
        data

      {category, pid} ->
        data = do_unregister(data, category, pid)
        %{data | monitors: Map.delete(data.monitors, ref)}
    end
  end

  defp format_error(%{__exception__: true} = exception) do
    Exception.message(exception)
  end

  defp format_error(%{reason: reason}) do
    format_error(reason)
  end

  defp format_error(:econnrefused), do: "connection refused"
  defp format_error(:closed), do: "connection closed"
  defp format_error(:timeout), do: "connection timeout"
  defp format_error(:nxdomain), do: "host not found"

  defp format_error(error) when is_binary(error), do: error
  defp format_error(error) when is_atom(error), do: Atom.to_string(error)
  defp format_error(error), do: inspect(error)

  # Filter out log effects when in quiet mode
  defp maybe_quiet(effects, %{config: %{quiet: true}}), do: Enum.reject(effects, &match?({:log, _, _}, &1))
  defp maybe_quiet(effects, _data), do: effects
end
