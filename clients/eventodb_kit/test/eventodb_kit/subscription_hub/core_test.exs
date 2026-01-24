defmodule EventodbKit.SubscriptionHub.CoreTest do
  use ExUnit.Case, async: true

  alias EventodbKit.SubscriptionHub.Core

  # Test helpers

  defp new_core(opts \\ []) do
    defaults = [
      kit_fn: fn -> :mock_kit end,
      now_fn: fn -> 1000 end,
      reconnect_base_delay: 1_000,
      reconnect_max_delay: 30_000,
      health_check_interval: 30_000,
      fallback_poll_interval: 5_000
    ]

    Core.new(Keyword.merge(defaults, opts))
  end

  defp register(data, category, pid) do
    {:connecting, data, _effects} =
      Core.handle_event(:connecting, data, {:register, :from, category, pid})

    # Simulate monitor being set up by shell
    ref = make_ref()
    %{data | monitors: Map.put(data.monitors, ref, {category, pid})}
  end

  defp find_effect(effects, type) do
    Enum.find(effects, fn
      {^type, _} -> true
      {^type, _, _} -> true
      _ -> false
    end)
  end

  defp has_effect?(effects, effect) do
    Enum.member?(effects, effect)
  end

  # ==========================================================================
  # Core.new/1
  # ==========================================================================

  describe "new/1" do
    test "creates core with default config" do
      data = Core.new()

      assert data.config.fallback_poll_interval == 5_000
      assert data.config.health_check_interval == 30_000
      assert data.config.reconnect_base_delay == 1_000
      assert data.config.reconnect_max_delay == 30_000
      assert data.registry == %{}
      assert data.monitors == %{}
      assert data.reconnect_attempts == 0
    end

    test "creates core with custom config" do
      data =
        Core.new(
          fallback_poll_interval: 1_000,
          health_check_interval: 10_000,
          reconnect_base_delay: 500,
          reconnect_max_delay: 5_000
        )

      assert data.config.fallback_poll_interval == 1_000
      assert data.config.health_check_interval == 10_000
      assert data.config.reconnect_base_delay == 500
      assert data.config.reconnect_max_delay == 5_000
    end

    test "accepts kit_fn" do
      kit_fn = fn -> :my_kit end
      data = Core.new(kit_fn: kit_fn)

      assert data.kit_fn.() == :my_kit
    end

    test "accepts custom now_fn for testing" do
      now_fn = fn -> 42 end
      data = Core.new(now_fn: now_fn)

      assert data.now_fn.() == 42
    end
  end

  # ==========================================================================
  # State: :connecting
  # ==========================================================================

  describe ":connecting state" do
    test "enter starts subscription" do
      data = new_core()

      {:connecting, _data, effects} = Core.handle_event(:connecting, data, :enter)

      assert {:start_subscription, kit_fn} = find_effect(effects, :start_subscription)
      assert kit_fn.() == :mock_kit
    end

    test "subscription_started transitions to :connected" do
      data = new_core()

      {state, data, effects} = Core.handle_event(:connecting, data, {:subscription_started, :sub_ref})

      assert state == :connected
      assert data.subscription_ref == :sub_ref
      assert data.reconnect_attempts == 0
      assert data.last_poke_at == 1000
      # Only log effects expected
      assert Enum.all?(effects, fn {type, _, _} -> type == :log end)
    end

    test "subscription_started resets reconnect_attempts" do
      data = %{new_core() | reconnect_attempts: 5}

      {_state, data, _effects} = Core.handle_event(:connecting, data, {:subscription_started, :ref})

      assert data.reconnect_attempts == 0
    end

    test "subscription_failed transitions to :disconnected with reconnect scheduled" do
      data = new_core()

      {state, data, effects} = Core.handle_event(:connecting, data, {:subscription_failed, :econnrefused})

      assert state == :disconnected
      assert data.reconnect_attempts == 1
      assert has_effect?(effects, {:schedule, :reconnect, 1_000})
    end

    test "subscription_failed increments reconnect_attempts" do
      data = %{new_core() | reconnect_attempts: 2}

      {_state, data, _effects} = Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert data.reconnect_attempts == 3
    end

    test "register adds consumer to registry" do
      data = new_core()
      pid = self()

      {:connecting, data, effects} =
        Core.handle_event(:connecting, data, {:register, :from, "order", pid})

      assert MapSet.member?(data.registry["order"], pid)
      assert has_effect?(effects, {:reply, :from, :ok})
      assert has_effect?(effects, {:monitor, pid})
    end

    test "register same consumer twice doesn't duplicate" do
      data = new_core() |> register("order", self())

      {:connecting, data, effects} =
        Core.handle_event(:connecting, data, {:register, :from, "order", self()})

      assert MapSet.size(data.registry["order"]) == 1
      assert has_effect?(effects, {:reply, :from, :ok})
      # No new monitor effect
      refute find_effect(effects, :monitor)
    end

    test "unregister removes consumer from registry" do
      pid = self()
      data = new_core() |> register("order", pid)

      {:connecting, data, effects} =
        Core.handle_event(:connecting, data, {:unregister, :from, "order", pid})

      refute Map.has_key?(data.registry, "order")
      assert has_effect?(effects, {:reply, :from, :ok})
    end

    test "status returns :connecting" do
      data = new_core()

      {:connecting, _data, effects} = Core.handle_event(:connecting, data, {:status, :from})

      assert has_effect?(effects, {:reply, :from, :connecting})
    end

    test "consumer_down removes consumer and demonitors" do
      pid = self()
      ref = make_ref()
      data = new_core()
      data = %{data | registry: %{"order" => MapSet.new([pid])}, monitors: %{ref => {"order", pid}}}

      {:connecting, data, effects} = Core.handle_event(:connecting, data, {:consumer_down, ref})

      refute Map.has_key?(data.registry, "order")
      refute Map.has_key?(data.monitors, ref)
      assert has_effect?(effects, {:demonitor, ref})
    end
  end

  # ==========================================================================
  # State: :connected
  # ==========================================================================

  describe ":connected state" do
    test "enter schedules health check" do
      data = new_core()

      {:connected, _data, effects} = Core.handle_event(:connected, data, :enter)

      assert [{:schedule, :health_check, 30_000}] = effects
    end

    test "sse_poke updates last_poke_at" do
      time = 5000
      data = new_core(now_fn: fn -> time end)
      poke = %{stream: "order-123", global_position: 42}

      {:connected, data, _effects} = Core.handle_event(:connected, data, {:sse_poke, poke})

      assert data.last_poke_at == time
    end

    test "sse_poke broadcasts to registered consumers" do
      pid1 = spawn(fn -> :ok end)
      pid2 = spawn(fn -> :ok end)
      data = new_core() |> register("order", pid1) |> register("order", pid2)
      poke = %{stream: "order-123", global_position: 42}

      {:connected, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})

      assert {:send, ^pid1, {:poke, "order", ^poke}} = find_effect(effects, :send)
      send_effects = Enum.filter(effects, &match?({:send, _, _}, &1))
      assert length(send_effects) == 2
    end

    test "sse_poke ignores unregistered categories" do
      data = new_core() |> register("order", self())
      poke = %{stream: "payment-456", global_position: 42}

      {:connected, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})

      assert effects == []
    end

    test "sse_poke extracts category correctly" do
      data = new_core() |> register("order_item", self())
      poke = %{stream: "order_item-123", global_position: 42}

      {:connected, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})

      assert {:send, _, {:poke, "order_item", _}} = find_effect(effects, :send)
    end

    test "sse_error transitions to :disconnected" do
      data = %{new_core() | subscription_ref: :sub_ref}

      {state, _data, effects} = Core.handle_event(:connected, data, {:sse_error, :timeout})

      assert state == :disconnected
      assert has_effect?(effects, {:kill_subscription, :sub_ref})
      assert has_effect?(effects, {:cancel_timer, :health_check})
      assert has_effect?(effects, {:schedule, :reconnect, 0})
    end

    test "health_check reschedules when recent poke" do
      # last_poke_at = 1000, now = 2000, silence = 1000ms < threshold (60000ms)
      data = %{new_core(now_fn: fn -> 2000 end) | last_poke_at: 1000}

      {:connected, _data, effects} = Core.handle_event(:connected, data, :health_check)

      assert [{:schedule, :health_check, 30_000}] = effects
    end

    test "health_check transitions to :disconnected when no recent poke" do
      # last_poke_at = 1000, now = 100000, silence = 99000ms > threshold (60000ms)
      data = %{new_core(now_fn: fn -> 100_000 end) | last_poke_at: 1000, subscription_ref: :ref}

      {state, _data, effects} = Core.handle_event(:connected, data, :health_check)

      assert state == :disconnected
      assert has_effect?(effects, {:kill_subscription, :ref})
      assert has_effect?(effects, {:schedule, :reconnect, 0})
    end

    test "register works in connected state" do
      data = new_core()
      pid = self()

      {:connected, data, effects} =
        Core.handle_event(:connected, data, {:register, :from, "order", pid})

      assert MapSet.member?(data.registry["order"], pid)
      assert has_effect?(effects, {:reply, :from, :ok})
      assert has_effect?(effects, {:monitor, pid})
    end

    test "unregister works in connected state" do
      pid = self()
      data = new_core() |> register("order", pid)

      {:connected, data, effects} =
        Core.handle_event(:connected, data, {:unregister, :from, "order", pid})

      refute Map.has_key?(data.registry, "order")
      assert has_effect?(effects, {:reply, :from, :ok})
    end

    test "status returns :connected" do
      data = new_core()

      {:connected, _data, effects} = Core.handle_event(:connected, data, {:status, :from})

      assert has_effect?(effects, {:reply, :from, :connected})
    end

    test "consumer_down removes consumer" do
      pid = self()
      ref = make_ref()
      data = new_core()
      data = %{data | registry: %{"order" => MapSet.new([pid])}, monitors: %{ref => {"order", pid}}}

      {:connected, data, effects} = Core.handle_event(:connected, data, {:consumer_down, ref})

      refute Map.has_key?(data.registry, "order")
      assert has_effect?(effects, {:demonitor, ref})
    end
  end

  # ==========================================================================
  # State: :disconnected
  # ==========================================================================

  describe ":disconnected state" do
    test "enter schedules fallback poll" do
      data = new_core()

      {:disconnected, _data, effects} = Core.handle_event(:disconnected, data, :enter)

      assert has_effect?(effects, {:schedule, :fallback_poll, 5_000})
    end

    test "reconnect transitions to :connecting" do
      data = new_core()

      {state, _data, effects} = Core.handle_event(:disconnected, data, :reconnect)

      assert state == :connecting
      assert has_effect?(effects, {:cancel_timer, :fallback_poll})
      # Note: :start_subscription is triggered by :enter for :connecting, not by :reconnect
      # This avoids duplicate subscription starts
    end

    test "fallback_poll sends synthetic pokes to all registered consumers" do
      pid1 = spawn(fn -> :ok end)
      pid2 = spawn(fn -> :ok end)
      data = new_core() |> register("order", pid1) |> register("payment", pid2)

      {:disconnected, _data, effects} = Core.handle_event(:disconnected, data, :fallback_poll)

      send_effects = Enum.filter(effects, &match?({:send, _, _}, &1))
      assert length(send_effects) == 2

      # Check synthetic poke format
      {_, _, {:poke, "order", poke}} =
        Enum.find(send_effects, fn {_, pid, _} -> pid == pid1 end)

      assert poke.stream == "order-fallback"
      assert poke.global_position == :poll
    end

    test "fallback_poll reschedules itself" do
      data = new_core()

      {:disconnected, _data, effects} = Core.handle_event(:disconnected, data, :fallback_poll)

      assert {:schedule, :fallback_poll, 5_000} = find_effect(effects, :schedule)
    end

    test "register works in disconnected state" do
      data = new_core()
      pid = self()

      {:disconnected, data, effects} =
        Core.handle_event(:disconnected, data, {:register, :from, "order", pid})

      assert MapSet.member?(data.registry["order"], pid)
      assert has_effect?(effects, {:reply, :from, :ok})
    end

    test "unregister works in disconnected state" do
      pid = self()
      data = new_core() |> register("order", pid)

      {:disconnected, data, effects} =
        Core.handle_event(:disconnected, data, {:unregister, :from, "order", pid})

      refute Map.has_key?(data.registry, "order")
      assert has_effect?(effects, {:reply, :from, :ok})
    end

    test "status returns :disconnected" do
      data = new_core()

      {:disconnected, _data, effects} = Core.handle_event(:disconnected, data, {:status, :from})

      assert has_effect?(effects, {:reply, :from, :disconnected})
    end

    test "consumer_down removes consumer" do
      pid = self()
      ref = make_ref()
      data = new_core()
      data = %{data | registry: %{"order" => MapSet.new([pid])}, monitors: %{ref => {"order", pid}}}

      {:disconnected, data, effects} = Core.handle_event(:disconnected, data, {:consumer_down, ref})

      refute Map.has_key?(data.registry, "order")
      assert has_effect?(effects, {:demonitor, ref})
    end
  end

  # ==========================================================================
  # Exponential Backoff
  # ==========================================================================

  describe "exponential backoff" do
    test "first failure uses base delay" do
      data = new_core()

      {_, data, effects} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 1_000} = find_effect(effects, :schedule)
      assert data.reconnect_attempts == 1
    end

    test "delay doubles with each attempt" do
      data = new_core()

      # 1st failure: 1000ms
      {_, data, effects1} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 1_000} = find_effect(effects1, :schedule)

      # 2nd failure: 2000ms
      {_, data, effects2} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 2_000} = find_effect(effects2, :schedule)

      # 3rd failure: 4000ms
      {_, data, effects3} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 4_000} = find_effect(effects3, :schedule)

      # 4th failure: 8000ms
      {_, _data, effects4} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 8_000} = find_effect(effects4, :schedule)
    end

    test "delay caps at max" do
      data = %{new_core() | reconnect_attempts: 10}

      {_, _, effects} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 30_000} = find_effect(effects, :schedule)
    end

    test "reconnect_delay helper function" do
      data = new_core()

      assert Core.reconnect_delay(%{data | reconnect_attempts: 0}) == 1_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 1}) == 2_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 2}) == 4_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 3}) == 8_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 4}) == 16_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 5}) == 30_000
      assert Core.reconnect_delay(%{data | reconnect_attempts: 10}) == 30_000
    end
  end

  # ==========================================================================
  # Category Extraction
  # ==========================================================================

  describe "extract_category/1" do
    test "extracts category from stream with id" do
      assert Core.extract_category("order-123") == "order"
      assert Core.extract_category("order-abc-def") == "order"
      assert Core.extract_category("order_item-456") == "order_item"
    end

    test "handles stream without id" do
      assert Core.extract_category("order") == "order"
    end

    test "handles atom stream names" do
      assert Core.extract_category(:"order-123") == "order"
    end
  end

  # ==========================================================================
  # Multiple Consumers
  # ==========================================================================

  describe "multiple consumers" do
    test "multiple consumers for same category all receive pokes" do
      pid1 = spawn(fn -> :ok end)
      pid2 = spawn(fn -> :ok end)
      pid3 = spawn(fn -> :ok end)

      data =
        new_core()
        |> register("order", pid1)
        |> register("order", pid2)
        |> register("order", pid3)

      poke = %{stream: "order-123", global_position: 42}
      {:connected, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, poke})

      send_effects = Enum.filter(effects, &match?({:send, _, _}, &1))
      assert length(send_effects) == 3

      pids_notified = Enum.map(send_effects, fn {:send, pid, _} -> pid end) |> MapSet.new()
      assert MapSet.equal?(pids_notified, MapSet.new([pid1, pid2, pid3]))
    end

    test "consumers for different categories only receive relevant pokes" do
      order_pid = spawn(fn -> :ok end)
      payment_pid = spawn(fn -> :ok end)

      data =
        new_core()
        |> register("order", order_pid)
        |> register("payment", payment_pid)

      # Order poke
      order_poke = %{stream: "order-123", global_position: 42}
      {:connected, _data, effects} = Core.handle_event(:connected, data, {:sse_poke, order_poke})

      assert [{:send, ^order_pid, {:poke, "order", ^order_poke}}] = effects
    end

    test "unregistering one consumer doesn't affect others" do
      pid1 = spawn(fn -> :ok end)
      pid2 = spawn(fn -> :ok end)

      data =
        new_core()
        |> register("order", pid1)
        |> register("order", pid2)

      {:connecting, data, _} =
        Core.handle_event(:connecting, data, {:unregister, :from, "order", pid1})

      assert MapSet.size(data.registry["order"]) == 1
      assert MapSet.member?(data.registry["order"], pid2)
    end
  end

  # ==========================================================================
  # Late/Stale Events (ignored)
  # ==========================================================================

  describe "late subscription events" do
    test "connected state ignores late subscription_started" do
      data = %{new_core() | subscription_ref: :current_ref}

      {:connected, data, effects} =
        Core.handle_event(:connected, data, {:subscription_started, :late_ref})

      # Should be ignored - no state change, no effects
      assert data.subscription_ref == :current_ref
      assert effects == []
    end

    test "connected state ignores late subscription_failed" do
      data = %{new_core() | subscription_ref: :current_ref}

      {:connected, data, effects} =
        Core.handle_event(:connected, data, {:subscription_failed, :some_error})

      assert data.subscription_ref == :current_ref
      assert effects == []
    end

    test "disconnected state ignores late subscription_started" do
      data = new_core()

      {:disconnected, _data, effects} =
        Core.handle_event(:disconnected, data, {:subscription_started, :late_ref})

      assert effects == []
    end

    test "disconnected state ignores late subscription_failed" do
      data = new_core()

      {:disconnected, _data, effects} =
        Core.handle_event(:disconnected, data, {:subscription_failed, :some_error})

      assert effects == []
    end
  end

  # ==========================================================================
  # State Transitions Summary
  # ==========================================================================

  describe "state transition flows" do
    test "happy path: connecting -> connected" do
      data = new_core()

      # Enter connecting
      {:connecting, data, effects} = Core.handle_event(:connecting, data, :enter)
      assert {:start_subscription, _} = find_effect(effects, :start_subscription)

      # Subscription succeeds
      {state, data, _effects} = Core.handle_event(:connecting, data, {:subscription_started, :ref})
      assert state == :connected
      assert data.subscription_ref == :ref
    end

    test "failure path: connecting -> disconnected -> connecting -> connected" do
      data = new_core()

      # Enter connecting
      {:connecting, data, _} = Core.handle_event(:connecting, data, :enter)

      # Subscription fails
      {:disconnected, data, effects} =
        Core.handle_event(:connecting, data, {:subscription_failed, :error})

      assert {:schedule, :reconnect, 1_000} = find_effect(effects, :schedule)

      # Enter disconnected
      {:disconnected, data, effects} = Core.handle_event(:disconnected, data, :enter)
      assert {:schedule, :fallback_poll, _} = find_effect(effects, :schedule)

      # Reconnect timer fires - transitions to :connecting
      {:connecting, data, effects} = Core.handle_event(:disconnected, data, :reconnect)
      assert has_effect?(effects, {:cancel_timer, :fallback_poll})

      # Enter connecting starts subscription
      {:connecting, data, effects} = Core.handle_event(:connecting, data, :enter)
      assert {:start_subscription, _} = find_effect(effects, :start_subscription)

      # This time it succeeds
      {:connected, data, _} = Core.handle_event(:connecting, data, {:subscription_started, :ref})
      assert data.reconnect_attempts == 0
    end

    test "SSE error path: connected -> disconnected -> connecting" do
      data = %{new_core() | subscription_ref: :old_ref, last_poke_at: 1000}

      # SSE error
      {:disconnected, data, effects} = Core.handle_event(:connected, data, {:sse_error, :closed})
      assert has_effect?(effects, {:kill_subscription, :old_ref})
      assert has_effect?(effects, {:schedule, :reconnect, 0})

      # Immediate reconnect - transitions to :connecting
      {:connecting, data, effects} = Core.handle_event(:disconnected, data, :reconnect)
      assert has_effect?(effects, {:cancel_timer, :fallback_poll})

      # Enter connecting starts subscription
      {:connecting, _data, effects} = Core.handle_event(:connecting, data, :enter)
      assert {:start_subscription, _} = find_effect(effects, :start_subscription)
    end

    test "health check timeout path: connected -> disconnected" do
      data = %{
        new_core(now_fn: fn -> 100_000 end)
        | subscription_ref: :ref,
          last_poke_at: 1000
      }

      {:disconnected, _data, effects} = Core.handle_event(:connected, data, :health_check)
      assert has_effect?(effects, {:kill_subscription, :ref})
      assert has_effect?(effects, {:schedule, :reconnect, 0})
    end
  end
end
