defmodule EventodbKit.SubscriptionHubTest do
  use ExUnit.Case, async: true

  alias EventodbKit.SubscriptionHub

  # These tests verify the gen_statem shell behavior with simulated SSE events.
  # The Core logic is tested exhaustively in CoreTest - here we just verify
  # the shell correctly translates messages and executes effects.

  describe "start_link/1" do
    test "starts and enters :connecting state" do
      # Use a kit_fn that will fail to connect (we'll send the failure manually)
      hub = start_hub!()

      # Should be in connecting state initially
      # (subscription will fail because kit_fn returns a mock)
      assert SubscriptionHub.status(hub) == :connecting
    end
  end

  describe "register/3" do
    test "registers consumer and they receive pokes" do
      hub = start_hub!()

      # Register self for "order" category
      :ok = SubscriptionHub.register(hub, "order", self())

      # Simulate successful connection
      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      # Simulate incoming poke
      poke = %{stream: "order-123", global_position: 42, position: 5}
      send(hub, {:sse_poke, poke})

      # Should receive the poke
      assert_receive {:poke, "order", ^poke}, 100
    end

    test "consumers for different categories receive only their pokes" do
      hub = start_hub!()

      # Start a separate process for payment
      payment_pid = spawn_link(fn -> consumer_loop() end)

      :ok = SubscriptionHub.register(hub, "order", self())
      :ok = SubscriptionHub.register(hub, "payment", payment_pid)

      # Connect
      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      # Send order poke
      order_poke = %{stream: "order-123", global_position: 42, position: 5}
      send(hub, {:sse_poke, order_poke})

      # Self should receive order poke
      assert_receive {:poke, "order", ^order_poke}, 100

      # Self should NOT receive payment pokes (none sent)
      refute_receive {:poke, "payment", _}, 50
    end

    test "multiple consumers for same category all receive pokes" do
      hub = start_hub!()
      test_pid = self()

      # Consumers that forward messages to test process
      consumer1 = spawn_link(fn -> forward_loop(test_pid, :c1) end)
      consumer2 = spawn_link(fn -> forward_loop(test_pid, :c2) end)

      :ok = SubscriptionHub.register(hub, "order", consumer1)
      :ok = SubscriptionHub.register(hub, "order", consumer2)

      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      poke = %{stream: "order-456", global_position: 100, position: 10}
      send(hub, {:sse_poke, poke})

      # Both should receive
      assert_receive {:from, :c1, {:poke, "order", ^poke}}, 100
      assert_receive {:from, :c2, {:poke, "order", ^poke}}, 100
    end
  end

  describe "unregister/3" do
    test "unregistered consumer no longer receives pokes" do
      hub = start_hub!()

      :ok = SubscriptionHub.register(hub, "order", self())

      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      # Unregister
      :ok = SubscriptionHub.unregister(hub, "order", self())

      # Send poke
      poke = %{stream: "order-123", global_position: 42, position: 5}
      send(hub, {:sse_poke, poke})

      # Should NOT receive
      refute_receive {:poke, "order", _}, 50
    end
  end

  describe "consumer process death" do
    test "dead consumer is automatically unregistered" do
      hub = start_hub!()

      # Start a consumer that will die
      consumer =
        spawn(fn ->
          receive do
            :die -> :ok
          end
        end)

      :ok = SubscriptionHub.register(hub, "order", consumer)

      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      # Kill the consumer
      send(consumer, :die)
      Process.sleep(50)

      # Send poke - should not crash (consumer cleaned up)
      poke = %{stream: "order-123", global_position: 42, position: 5}
      send(hub, {:sse_poke, poke})

      # Hub should still be alive
      assert Process.alive?(hub)
    end
  end

  describe "SSE error handling" do
    test "SSE error transitions to :disconnected" do
      hub = start_hub!()

      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      # Simulate SSE error
      send(hub, {:sse_error, :connection_closed})

      wait_for_state(hub, :disconnected)
    end

    test "fallback polling sends synthetic pokes when disconnected" do
      hub = start_hub!(fallback_poll_interval: 50)

      :ok = SubscriptionHub.register(hub, "order", self())

      # Fail to connect -> disconnected
      send(hub, {:subscription_result, {:error, :econnrefused}})
      wait_for_state(hub, :disconnected)

      # Should receive fallback poll poke
      assert_receive {:poke, "order", %{global_position: :poll}}, 200
    end
  end

  describe "reconnection" do
    test "reconnects after disconnection" do
      hub = start_hub!(reconnect_base_delay: 10, reconnect_max_delay: 50)

      # Connect then disconnect
      send(hub, {:subscription_result, {:ok, :fake_ref}})
      wait_for_state(hub, :connected)

      send(hub, {:sse_error, :timeout})
      wait_for_state(hub, :disconnected)

      # Wait for reconnect timer (very short in test)
      Process.sleep(100)

      # Should be trying to connect again or already connected
      # (if EventoDB is running, it will connect immediately)
      status = SubscriptionHub.status(hub)
      assert status in [:connecting, :connected]
    end
  end

  describe "status/1" do
    test "returns current state" do
      hub = start_hub!()

      assert SubscriptionHub.status(hub) == :connecting

      send(hub, {:subscription_result, {:ok, :ref}})
      wait_for_state(hub, :connected)
      assert SubscriptionHub.status(hub) == :connected

      send(hub, {:sse_error, :closed})
      wait_for_state(hub, :disconnected)
      assert SubscriptionHub.status(hub) == :disconnected
    end
  end

  # ==========================================================================
  # Helpers
  # ==========================================================================

  defp start_hub!(opts \\ []) do
    defaults = [
      kit_fn: fn -> mock_kit() end,
      fallback_poll_interval: 5_000,
      health_check_interval: 30_000,
      reconnect_base_delay: 1_000,
      reconnect_max_delay: 30_000
    ]

    opts = Keyword.merge(defaults, opts)

    # Generate unique name
    name = :"hub_#{System.unique_integer([:positive])}"
    opts = Keyword.put(opts, :name, name)

    {:ok, pid} = SubscriptionHub.start_link(opts)
    pid
  end

  defp mock_kit do
    # Return a struct that looks like EventodbKit.Client
    %{
      eventodb_client: %{
        base_url: "http://localhost:8080",
        token: "test_token"
      }
    }
  end

  defp wait_for_state(hub, expected_state, timeout \\ 500) do
    deadline = System.monotonic_time(:millisecond) + timeout

    wait_loop(hub, expected_state, deadline)
  end

  defp wait_loop(hub, expected_state, deadline) do
    if System.monotonic_time(:millisecond) > deadline do
      actual = SubscriptionHub.status(hub)
      flunk("Timeout waiting for state #{expected_state}, got #{actual}")
    end

    if SubscriptionHub.status(hub) == expected_state do
      :ok
    else
      Process.sleep(10)
      wait_loop(hub, expected_state, deadline)
    end
  end

  defp consumer_loop do
    receive do
      msg ->
        # Store message for later retrieval
        send(self(), {:received, msg})
        consumer_loop()
    end
  end

  defp forward_loop(test_pid, tag) do
    receive do
      msg ->
        send(test_pid, {:from, tag, msg})
        forward_loop(test_pid, tag)
    end
  end
end
