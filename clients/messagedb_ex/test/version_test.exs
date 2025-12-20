defmodule MessagedbEx.VersionTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("version")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "VERSION-001: Version of non-existent stream", %{client: client} do
    stream = unique_stream()

    assert {:ok, nil, _client} = MessagedbEx.stream_version(client, stream)
  end

  test "VERSION-002: Version of stream with messages", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 3 messages (positions 0, 1, 2)
    client =
      Enum.reduce(1..3, client, fn _, c ->
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    assert {:ok, 2, _client} = MessagedbEx.stream_version(client, stream)
  end

  test "VERSION-003: Version after write", %{client: client} do
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    # Write 1 message
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)

    # Version should be 0
    {:ok, version, client} = MessagedbEx.stream_version(client, stream)
    assert version == 0

    # Write another message
    {:ok, _result, client} = MessagedbEx.stream_write(client, stream, message)

    # Version should be 1
    {:ok, version, _client} = MessagedbEx.stream_version(client, stream)
    assert version == 1
  end
end
