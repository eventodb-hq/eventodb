defmodule MessagedbEx.LastTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("last")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "LAST-001: Last message from non-empty stream", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 5 messages
    client =
      Enum.reduce(1..5, client, fn _, c ->
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, last_msg, _client} = MessagedbEx.stream_last(client, stream)
    [_id, _type, position, _gpos, _data, _metadata, _time] = last_msg

    assert position == 4
  end

  test "LAST-002: Last message from empty stream", %{client: client} do
    stream = unique_stream()

    assert {:ok, nil, _client} = MessagedbEx.stream_last(client, stream)
  end

  test "LAST-003: Last message filtered by type", %{client: client} do
    stream = unique_stream()

    # Write messages: TypeA, TypeB, TypeA, TypeB, TypeA
    types = ["TypeA", "TypeB", "TypeA", "TypeB", "TypeA"]

    client =
      Enum.reduce(types, client, fn type, c ->
        message = %{type: type, data: %{}}
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, last_msg, _client} = MessagedbEx.stream_last(client, stream, %{type: "TypeB"})
    [_id, type, position, _gpos, _data, _metadata, _time] = last_msg

    assert type == "TypeB"
    assert position == 3
  end

  test "LAST-004: Last message type filter no match", %{client: client} do
    stream = unique_stream()

    # Write only TypeA messages
    client =
      Enum.reduce(1..3, client, fn _, c ->
        message = %{type: "TypeA", data: %{}}
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    assert {:ok, nil, _client} = MessagedbEx.stream_last(client, stream, %{type: "TypeB"})
  end
end
