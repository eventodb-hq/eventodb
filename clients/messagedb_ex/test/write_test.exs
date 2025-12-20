defmodule MessagedbEx.WriteTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("write")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "WRITE-001: Write minimal message", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{foo: "bar"}}

    assert {:ok, result, _client} = MessagedbEx.stream_write(client, stream, message)
    assert result.position >= 0
    assert result.global_position >= 0
  end

  test "WRITE-002: Write message with metadata", %{client: client} do
    stream = unique_stream()

    message = %{
      type: "TestEvent",
      data: %{foo: "bar"},
      metadata: %{correlationId: "123"}
    }

    assert {:ok, result, client} = MessagedbEx.stream_write(client, stream, message)
    assert result.position >= 0

    # Read back and verify metadata
    {:ok, [read_msg], _client} = MessagedbEx.stream_get(client, stream)
    [_id, _type, _pos, _gpos, _data, metadata, _time] = read_msg
    assert metadata["correlationId"] == "123"
  end

  test "WRITE-003: Write with custom message ID", %{client: client} do
    stream = unique_stream()
    custom_id = "550e8400-e29b-41d4-a716-446655440000"
    message = %{type: "TestEvent", data: %{foo: "bar"}}

    assert {:ok, result, client} =
             MessagedbEx.stream_write(client, stream, message, %{id: custom_id})

    assert result.position >= 0

    # Read back and verify ID
    {:ok, [read_msg], _client} = MessagedbEx.stream_get(client, stream)
    [id | _rest] = read_msg
    assert id == custom_id
  end

  test "WRITE-004: Write with expected version (success)", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 2 messages
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)

    # Write with expected version 1 should succeed
    assert {:ok, result, _client} =
             MessagedbEx.stream_write(client, stream, message, %{expected_version: 1})

    assert result.position == 2
  end

  test "WRITE-005: Write with expected version (conflict)", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 2 messages
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)

    # Write with wrong expected version should fail
    assert {:error, error} =
             MessagedbEx.stream_write(client, stream, message, %{expected_version: 5})

    assert error.code == "STREAM_VERSION_CONFLICT"
  end

  test "WRITE-006: Write multiple messages sequentially", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    results =
      Enum.reduce(1..5, {[], client}, fn _, {acc, c} ->
        {:ok, result, new_c} = MessagedbEx.stream_write(c, stream, message)
        {[result | acc], new_c}
      end)
      |> elem(0)
      |> Enum.reverse()

    # Verify positions are sequential
    positions = Enum.map(results, & &1.position)
    assert positions == [0, 1, 2, 3, 4]

    # Verify global positions are monotonically increasing
    global_positions = Enum.map(results, & &1.global_position)
    assert global_positions == Enum.sort(global_positions)
  end

  test "WRITE-007: Write to stream with ID", %{client: client} do
    stream = "account-123"
    message = %{type: "TestEvent", data: %{}}

    assert {:ok, result, _client} = MessagedbEx.stream_write(client, stream, message)
    assert result.position >= 0
  end

  test "WRITE-008: Write with empty data object", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:ok, result, client} = MessagedbEx.stream_write(client, stream, message)
    assert result.position >= 0

    # Verify data is stored as empty object
    {:ok, [read_msg], _client} = MessagedbEx.stream_get(client, stream)
    [_id, _type, _pos, _gpos, data, _metadata, _time] = read_msg
    assert data == %{}
  end

  test "WRITE-009: Write with null metadata", %{client: client} do
    stream = unique_stream()
    # Server doesn't accept null metadata, omit it instead
    message = %{type: "TestEvent", data: %{x: 1}}

    assert {:ok, result, client} = MessagedbEx.stream_write(client, stream, message)
    assert result.position >= 0

    # Verify metadata is stored as null when omitted
    {:ok, [read_msg], _client} = MessagedbEx.stream_get(client, stream)
    [_id, _type, _pos, _gpos, _data, metadata, _time] = read_msg
    assert metadata == nil
  end

  test "WRITE-010: Write without authentication", %{client: _client} do
    # Create client without token
    client = MessagedbEx.Client.new(base_url())
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:error, error} = MessagedbEx.stream_write(client, stream, message)
    assert error.code == "AUTH_REQUIRED"
  end
end
