defmodule MessagedbEx.AuthTest do
  use ExUnit.Case, async: true
  import MessagedbEx.TestHelper

  test "AUTH-001: Valid token authentication" do
    {client, namespace_id, _token} = create_test_namespace("auth")

    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:ok, _result, _client} = MessagedbEx.stream_write(client, stream, message)

    # Cleanup
    cleanup_namespace(client, namespace_id)
  end

  test "AUTH-002: Missing token" do
    client = MessagedbEx.Client.new(base_url())
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:error, error} = MessagedbEx.stream_write(client, stream, message)
    assert error.code == "AUTH_REQUIRED"
  end

  test "AUTH-003: Invalid token format" do
    client = MessagedbEx.Client.new(base_url(), token: "invalid-token")
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:error, error} = MessagedbEx.stream_write(client, stream, message)
    assert error.code in ["AUTH_INVALID", "AUTH_REQUIRED", "AUTH_INVALID_TOKEN"]
  end

  test "AUTH-004: Token namespace isolation" do
    # Create two namespaces
    {client1, namespace_id1, _token1} = create_test_namespace("auth-iso1")
    {client2, namespace_id2, _token2} = create_test_namespace("auth-iso2")

    stream = "shared-stream-name"
    message = %{type: "TestEvent", data: %{value: 1}}

    # Write to ns1
    {:ok, _result, _client1} = MessagedbEx.stream_write(client1, stream, message)

    # Try to read from ns2 using same stream name
    {:ok, messages, _client2} = MessagedbEx.stream_get(client2, stream)

    # Should not see messages from ns1
    assert messages == []

    # Cleanup
    cleanup_namespace(client1, namespace_id1)
    cleanup_namespace(client2, namespace_id2)
  end
end
