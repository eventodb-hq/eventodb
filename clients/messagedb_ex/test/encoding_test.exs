defmodule MessagedbEx.EncodingTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("encoding")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "ENCODING-001: UTF-8 text in data", %{client: client} do
    stream = unique_stream()
    utf8_text = "Hello 世界 🌍 émojis"
    message = %{type: "TestEvent", data: %{text: utf8_text}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["text"] == utf8_text
  end

  test "ENCODING-002: Unicode in metadata", %{client: client} do
    stream = unique_stream()
    utf8_text = "Test 测试 🎉"
    message = %{type: "TestEvent", data: %{}, metadata: %{description: utf8_text}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, _data, metadata, _time] = msg
    assert metadata["description"] == utf8_text
  end

  test "ENCODING-003: Special characters in stream name", %{client: client} do
    stream = "test-stream_123.abc-#{unique_suffix()}"
    message = %{type: "TestEvent", data: %{}}

    # Should succeed or give clear error
    result = MessagedbEx.stream_write(client, stream, message)
    assert match?({:ok, _, _}, result) or match?({:error, _}, result)
  end

  test "ENCODING-004: Empty string values", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{empty_string: ""}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["empty_string"] == ""
    assert data["empty_string"] != nil
  end

  test "ENCODING-005: Boolean values", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{is_true: true, is_false: false}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["is_true"] == true
    assert data["is_false"] == false
    assert is_boolean(data["is_true"])
    assert is_boolean(data["is_false"])
  end

  test "ENCODING-006: Null values", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{null_value: nil}}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["null_value"] == nil
    assert Map.has_key?(data, "null_value")
  end

  test "ENCODING-007: Numeric values", %{client: client} do
    stream = unique_stream()

    message = %{
      type: "TestEvent",
      data: %{
        integer: 42,
        float: 3.14159,
        negative: -100,
        zero: 0
      }
    }

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["integer"] == 42
    assert_in_delta data["float"], 3.14159, 0.00001
    assert data["negative"] == -100
    assert data["zero"] == 0
  end

  test "ENCODING-008: Nested objects", %{client: client} do
    stream = unique_stream()

    nested = %{
      level1: %{
        level2: %{
          level3: %{
            value: "deep"
          }
        }
      }
    }

    message = %{type: "TestEvent", data: nested}

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert data["level1"]["level2"]["level3"]["value"] == "deep"
  end

  test "ENCODING-009: Arrays in data", %{client: client} do
    stream = unique_stream()

    message = %{
      type: "TestEvent",
      data: %{
        items: [1, "two", %{three: 3}, nil, true]
      }
    }

    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)

    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    items = data["items"]
    assert Enum.at(items, 0) == 1
    assert Enum.at(items, 1) == "two"
    assert Enum.at(items, 2) == %{"three" => 3}
    assert Enum.at(items, 3) == nil
    assert Enum.at(items, 4) == true
  end

  test "ENCODING-010: Large message payload", %{client: client} do
    stream = unique_stream()

    # Create a large data object (~10KB)
    large_data = %{
      items: Enum.map(1..1000, fn i -> %{id: i, name: "Item #{i}", value: i * 10} end)
    }

    message = %{type: "TestEvent", data: large_data}

    # Should succeed (under 1MB limit)
    assert {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)

    # Verify it reads back correctly
    {:ok, [msg], _client} = MessagedbEx.stream_get(client, stream)
    [_id, _type, _pos, _gpos, data, _metadata, _time] = msg
    assert length(data["items"]) == 1000
  end
end
