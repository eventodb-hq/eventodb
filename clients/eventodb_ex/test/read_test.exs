defmodule EventodbEx.ReadTest do
  use ExUnit.Case, async: true
  import EventodbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("read")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "READ-001: Read from empty stream", %{client: client} do
    stream = unique_stream()

    assert {:ok, messages, _client} = EventodbEx.stream_get(client, stream)
    assert messages == []
  end

  test "READ-002: Read single message", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{foo: "bar"}}

    {:ok, _result, client} = EventodbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = EventodbEx.stream_get(client, stream)

    assert length(msg) == 7
    [id, type, position, global_position, data, metadata, time] = msg

    assert is_binary(id)
    assert type == "TestEvent"
    assert position == 0
    assert is_integer(global_position)
    assert data == %{"foo" => "bar"}
    assert is_map(metadata) or is_nil(metadata)
    assert is_binary(time)
  end

  test "READ-003: Read multiple messages", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 5 messages
    client =
      Enum.reduce(1..5, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = EventodbEx.stream_get(client, stream)
    assert length(messages) == 5

    # Verify positions are in order
    positions = Enum.map(messages, fn [_, _, pos, _, _, _, _] -> pos end)
    assert positions == [0, 1, 2, 3, 4]
  end

  test "READ-004: Read with position filter", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 10 messages
    client =
      Enum.reduce(1..10, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = EventodbEx.stream_get(client, stream, %{position: 5})
    assert length(messages) == 5

    positions = Enum.map(messages, fn [_, _, pos, _, _, _, _] -> pos end)
    assert positions == [5, 6, 7, 8, 9]
  end

  test "READ-005: Read with global position filter", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write messages and track global positions
    results =
      Enum.reduce(1..4, {[], client}, fn _, {acc, c} ->
        {:ok, result, new_c} = EventodbEx.stream_write(c, stream, message)
        {[result | acc], new_c}
      end)
      |> elem(0)
      |> Enum.reverse()

    # Get the third message's global position
    target_gpos = Enum.at(results, 2).global_position

    {:ok, messages, _client} =
      EventodbEx.stream_get(client, stream, %{global_position: target_gpos})

    global_positions = Enum.map(messages, fn [_, _, _, gpos, _, _, _] -> gpos end)
    assert Enum.all?(global_positions, &(&1 >= target_gpos))
  end

  test "READ-006: Read with batch size limit", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 100 messages
    client =
      Enum.reduce(1..100, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = EventodbEx.stream_get(client, stream, %{batch_size: 10})
    assert length(messages) == 10

    positions = Enum.map(messages, fn [_, _, pos, _, _, _, _] -> pos end)
    assert positions == Enum.to_list(0..9)
  end

  test "READ-007: Read with batch size unlimited", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 50 messages
    client =
      Enum.reduce(1..50, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = EventodbEx.stream_get(client, stream, %{batch_size: -1})
    assert length(messages) == 50
  end

  test "READ-008: Read message data integrity", %{client: client} do
    stream = unique_stream()

    complex_data = %{
      nested: %{
        array: [1, 2, 3],
        bool: true,
        null: nil
      }
    }

    message = %{type: "TestEvent", data: complex_data}

    {:ok, _result, client} = EventodbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = EventodbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg

    # Verify deep equality
    assert data["nested"]["array"] == [1, 2, 3]
    assert data["nested"]["bool"] == true
    assert data["nested"]["null"] == nil
  end

  test "READ-009: Read message metadata integrity", %{client: client} do
    stream = unique_stream()

    metadata = %{
      correlationId: "123",
      userId: "user-456"
    }

    message = %{type: "TestEvent", data: %{}, metadata: metadata}

    {:ok, _result, client} = EventodbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = EventodbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, _data, read_metadata, _time] = msg

    assert read_metadata["correlationId"] == "123"
    assert read_metadata["userId"] == "user-456"
  end

  test "READ-010: Read message timestamp format", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    {:ok, _result, client} = EventodbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = EventodbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, _data, _metadata, time] = msg

    # Verify ISO 8601 timestamp format (with or without nanoseconds)
    assert String.match?(time, ~r/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/)
  end
end
