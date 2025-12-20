defmodule MessagedbEx.CategoryTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("category")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "CATEGORY-001: Read from category", %{client: client} do
    # Use category without hyphens (per SDK-TEST-SPEC.md)
    category = "test#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Write messages to test123-1, test123-2, test123-3 streams
    client =
      Enum.reduce(1..3, client, fn i, c ->
        stream = "#{category}-#{i}"
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = MessagedbEx.category_get(client, category)
    assert length(messages) == 3
  end

  test "CATEGORY-002: Read category with position filter", %{client: client} do
    category = "test#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Write multiple messages and track global positions
    {results, client} =
      Enum.reduce(1..4, {[], client}, fn i, {acc, c} ->
        stream = "#{category}-#{i}"
        {:ok, result, new_c} = MessagedbEx.stream_write(c, stream, message)
        {[result | acc], new_c}
      end)

    results = Enum.reverse(results)
    target_gpos = Enum.at(results, 2).global_position

    {:ok, messages, _client} =
      MessagedbEx.category_get(client, category, %{position: target_gpos})

    global_positions = Enum.map(messages, fn [_, _, _, _, gpos, _, _, _] -> gpos end)
    assert Enum.all?(global_positions, &(&1 >= target_gpos))
  end

  test "CATEGORY-003: Read category with batch size", %{client: client} do
    category = "test#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Write 50 messages across multiple streams
    client =
      Enum.reduce(1..50, client, fn i, c ->
        stream = "#{category}-#{rem(i, 10)}"
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = MessagedbEx.category_get(client, category, %{batch_size: 10})
    assert length(messages) == 10
  end

  test "CATEGORY-004: Category message format", %{client: client} do
    category = "test#{unique_suffix()}"
    stream = "#{category}-123"
    message = %{type: "TestEvent", data: %{foo: "bar"}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.category_get(client, category)

    # Category message has 8 elements: [id, streamName, type, position, globalPosition, data, metadata, time]
    assert length(msg) == 8
    [id, stream_name, type, position, global_position, data, metadata, time] = msg

    assert is_binary(id)
    assert stream_name == stream
    assert type == "TestEvent"
    assert position == 0
    assert is_integer(global_position)
    assert data == %{"foo" => "bar"}
    assert is_map(metadata) or is_nil(metadata)
    assert is_binary(time)
  end

  test "CATEGORY-005: Category with consumer group", %{client: client} do
    category = "test#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Write messages to test123-1, test123-2, test123-3, test123-4
    client =
      Enum.reduce(1..4, client, fn i, c ->
        stream = "#{category}-#{i}"
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    # Get messages for member 0 of 2
    {:ok, messages_0, _client} =
      MessagedbEx.category_get(client, category, %{
        consumer_group: %{member: 0, size: 2}
      })

    # Get messages for member 1 of 2
    {:ok, messages_1, _client} =
      MessagedbEx.category_get(client, category, %{
        consumer_group: %{member: 1, size: 2}
      })

    # Both should have some messages and together cover all streams
    assert length(messages_0) > 0
    assert length(messages_1) > 0
    assert length(messages_0) + length(messages_1) == 4
  end

  test "CATEGORY-006: Category with correlation filter", %{client: client} do
    category = "test#{unique_suffix()}"

    # Write message to test123-1 with workflow correlation
    stream1 = "#{category}-1"

    message1 = %{
      type: "TestEvent",
      data: %{},
      metadata: %{correlationStreamName: "workflow-123"}
    }

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream1, message1)

    # Write message to test123-2 with other correlation
    stream2 = "#{category}-2"

    message2 = %{
      type: "TestEvent",
      data: %{},
      metadata: %{correlationStreamName: "other-456"}
    }

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream2, message2)

    # Filter by workflow correlation
    {:ok, messages, _client} =
      MessagedbEx.category_get(client, category, %{correlation: "workflow"})

    assert length(messages) == 1
    [_id, stream_name, _type, _pos, _gpos, _data, _metadata, _time] = List.first(messages)
    assert stream_name == stream1
  end

  test "CATEGORY-007: Read from empty category", %{client: client} do
    category = "nonexistent-#{unique_suffix()}"

    assert {:ok, [], _client} = MessagedbEx.category_get(client, category)
  end

  test "CATEGORY-008: Category global position ordering", %{client: client} do
    category = "test#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Write messages across multiple streams
    client =
      Enum.reduce(1..10, client, fn i, c ->
        stream = "#{category}-#{rem(i, 3)}"
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    {:ok, messages, _client} = MessagedbEx.category_get(client, category)

    # Verify messages are in ascending global position order
    global_positions = Enum.map(messages, fn [_, _, _, _, gpos, _, _, _] -> gpos end)
    assert global_positions == Enum.sort(global_positions)
  end
end
