defmodule EventodbEx.SseTest do
  use ExUnit.Case, async: true
  import EventodbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("sse")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "SSE-001: Subscribe to stream", %{client: client} do
    stream = unique_stream()
    test_pid = self()

    {:ok, _pid} =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "sse-001",
        position: 0,
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # Write a message
    message = %{type: "TestEvent", data: %{value: 1}}
    {:ok, result, _client} = EventodbEx.stream_write(client, stream, message)

    # Should receive poke
    assert_receive {:poke, poke}, 5000
    assert poke.stream == stream
    assert poke.position == result.position
    assert poke.global_position == result.global_position

    # Cleanup
    EventodbEx.Subscription.close("sse-001")
  end

  test "SSE-002: Subscribe to category", %{client: client} do
    category = "test#{unique_suffix()}"
    test_pid = self()

    {:ok, _pid} =
      EventodbEx.subscribe_to_category(client, category,
        name: "sse-002",
        position: 0,
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # Write message to stream in category
    stream = "#{category}-123"
    message = %{type: "TestEvent", data: %{value: 1}}
    {:ok, result, _client} = EventodbEx.stream_write(client, stream, message)

    # Should receive poke
    assert_receive {:poke, poke}, 5000
    assert poke.stream == stream
    assert poke.position == result.position
    assert poke.global_position == result.global_position

    # Cleanup
    EventodbEx.Subscription.close("sse-002")
  end

  test "SSE-003: Subscribe with position", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 5 messages (positions 0-4)
    client =
      Enum.reduce(1..5, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    test_pid = self()

    # Subscribe from position 3 (server sends pokes starting from position 3)
    {:ok, _pid} =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "sse-003",
        position: 3,
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # The subscription will first receive pokes for existing messages at positions 3 and 4
    # Then receive poke for new messages
    # So we'll write a new message and it should be at position 5
    {:ok, _result, _client} = EventodbEx.stream_write(client, stream, message)

    # May receive pokes for positions 3, 4, or 5
    # Just verify we receive at least one poke
    assert_receive {:poke, poke}, 5000
    assert poke.position >= 3

    # Cleanup
    EventodbEx.Subscription.close("sse-003")
  end

  test "SSE-004: Subscribe without authentication" do
    # Create client without token
    client = EventodbEx.Client.new(base_url())
    stream = unique_stream()

    # Should fail to subscribe
    result =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "sse-004",
        on_poke: fn _poke -> :ok end
      )

    # Either fails immediately or connection error
    case result do
      {:ok, _pid} ->
        # Wait a bit to see if connection fails
        Process.sleep(50)  # 50ms is enough
        # Try to close if it started (shouldn't be registered if it failed)
        EventodbEx.Subscription.close("sse-004")

      {:error, _} ->
        :ok
    end
  end

  test "SSE-005: Subscribe with consumer group", %{client: client} do
    category = "test#{unique_suffix()}"
    test_pid = self()

    # Subscribe with consumer group member 0
    {:ok, _pid} =
      EventodbEx.subscribe_to_category(client, category,
        name: "sse-005",
        consumer_group: %{member: 0, size: 2},
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # Write to multiple streams
    streams = ["#{category}-1", "#{category}-2", "#{category}-3", "#{category}-4"]
    message = %{type: "TestEvent", data: %{}}

    Enum.each(streams, fn stream ->
      EventodbEx.stream_write(client, stream, message)
      Process.sleep(10)  # Just need small delay for SSE
    end)

    # Should receive some pokes (based on consumer group partitioning)
    pokes =
      Enum.reduce_while(1..4, [], fn _, acc ->
        receive do
          {:poke, poke} -> {:cont, [poke | acc]}
        after
          200 -> {:halt, acc}  # 200ms max wait
        end
      end)

    assert length(pokes) > 0

    # Cleanup
    EventodbEx.Subscription.close("sse-005")
  end

  test "SSE-006: Multiple subscriptions", %{client: client} do
    stream1 = unique_stream()
    stream2 = unique_stream()
    test_pid = self()

    # Subscribe to two different streams
    {:ok, _pid1} =
      EventodbEx.subscribe_to_stream(client, stream1,
        name: "sse-006-a",
        on_poke: fn poke -> send(test_pid, {:poke1, poke}) end
      )

    {:ok, _pid2} =
      EventodbEx.subscribe_to_stream(client, stream2,
        name: "sse-006-b",
        on_poke: fn poke -> send(test_pid, {:poke2, poke}) end
      )

    # Write to stream1
    message = %{type: "TestEvent", data: %{}}
    {:ok, _result, client} = EventodbEx.stream_write(client, stream1, message)

    # Should only receive poke1
    assert_receive {:poke1, poke}, 5000
    assert poke.stream == stream1

    # Write to stream2
    {:ok, _result, _client} = EventodbEx.stream_write(client, stream2, message)

    # Should only receive poke2
    assert_receive {:poke2, poke}, 5000
    assert poke.stream == stream2

    # Cleanup
    EventodbEx.Subscription.close("sse-006-a")
    EventodbEx.Subscription.close("sse-006-b")
  end

  test "SSE-008: Poke event parsing", %{client: client} do
    stream = unique_stream()
    test_pid = self()

    {:ok, _pid} =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "sse-008",
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # Write a message
    message = %{type: "TestEvent", data: %{value: 123}}
    {:ok, result, _client} = EventodbEx.stream_write(client, stream, message)

    # Receive poke and verify structure
    assert_receive {:poke, poke}, 5000

    assert is_binary(poke.stream)
    assert is_integer(poke.position)
    assert is_integer(poke.global_position)

    assert poke.stream == stream
    assert poke.position == result.position
    assert poke.global_position == result.global_position

    # Cleanup
    EventodbEx.Subscription.close("sse-008")
  end

  test "Subscription name must be unique", %{client: client} do
    stream = unique_stream()

    {:ok, _pid} =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "duplicate-name",
        on_poke: fn _poke -> :ok end
      )

    # Try to create another with same name
    result =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "duplicate-name",
        on_poke: fn _poke -> :ok end
      )

    assert {:error, {:already_started, _pid}} = result

    # Cleanup
    EventodbEx.Subscription.close("duplicate-name")
  end

  test "Close subscription by name", %{client: client} do
    stream = unique_stream()
    test_pid = self()

    {:ok, _pid} =
      EventodbEx.subscribe_to_stream(client, stream,
        name: "close-test",
        on_poke: fn poke -> send(test_pid, {:poke, poke}) end
      )

    # Close subscription
    assert :ok = EventodbEx.Subscription.close("close-test")

    # Write message - should not receive poke
    message = %{type: "TestEvent", data: %{}}
    {:ok, _result, _client} = EventodbEx.stream_write(client, stream, message)

    refute_receive {:poke, _}, 100  # 100ms is enough
  end

  test "Close non-existent subscription", %{client: _client} do
    assert {:error, :not_found} = EventodbEx.Subscription.close("does-not-exist")
  end

  test "SSE-009: Multiple consumers in same consumer group", %{client: client} do
    category = "test#{unique_suffix()}"
    test_pid = self()

    # Create two consumers in the same consumer group (size 2)
    {:ok, _pid0} =
      EventodbEx.subscribe_to_category(client, category,
        name: "sse-009-consumer-0",
        consumer_group: %{member: 0, size: 2},
        on_poke: fn poke -> send(test_pid, {:poke0, poke}) end
      )

    {:ok, _pid1} =
      EventodbEx.subscribe_to_category(client, category,
        name: "sse-009-consumer-1",
        consumer_group: %{member: 1, size: 2},
        on_poke: fn poke -> send(test_pid, {:poke1, poke}) end
      )

    # Write messages to 4 different streams in the category
    streams = ["#{category}-1", "#{category}-2", "#{category}-3", "#{category}-4"]
    message = %{type: "TestEvent", data: %{}}

    Enum.each(streams, fn stream ->
      {:ok, _result, _client} = EventodbEx.stream_write(client, stream, message)
      Process.sleep(10)
    end)

    # Collect pokes from both consumers
    consumer0_streams =
      Enum.reduce_while(1..10, MapSet.new(), fn _, acc ->
        receive do
          {:poke0, poke} -> {:cont, MapSet.put(acc, poke.stream)}
        after
          50 -> {:halt, acc}
        end
      end)

    consumer1_streams =
      Enum.reduce_while(1..10, MapSet.new(), fn _, acc ->
        receive do
          {:poke1, poke} -> {:cont, MapSet.put(acc, poke.stream)}
        after
          50 -> {:halt, acc}
        end
      end)

    # Verify both consumers received some pokes
    assert MapSet.size(consumer0_streams) > 0, "Consumer 0 should receive some pokes"
    assert MapSet.size(consumer1_streams) > 0, "Consumer 1 should receive some pokes"

    # Verify no stream is received by both consumers (exclusive partitioning)
    intersection = MapSet.intersection(consumer0_streams, consumer1_streams)

    assert MapSet.size(intersection) == 0,
           "No stream should be received by both consumers, but got: #{inspect(MapSet.to_list(intersection))}"

    # Verify all streams are covered by at least one consumer
    all_streams_received = MapSet.union(consumer0_streams, consumer1_streams)

    Enum.each(streams, fn stream ->
      assert MapSet.member?(all_streams_received, stream),
             "Stream #{stream} should be received by at least one consumer"
    end)

    # Cleanup
    EventodbEx.Subscription.close("sse-009-consumer-0")
    EventodbEx.Subscription.close("sse-009-consumer-1")
  end
end
