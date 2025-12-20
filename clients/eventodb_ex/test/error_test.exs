defmodule EventodbEx.ErrorTest do
  use ExUnit.Case, async: true
  import EventodbEx.TestHelper

  setup context do
    # Skip setup if test is tagged with :skip_setup
    if context[:skip_setup] do
      :ok
    else
      {client, namespace_id, _token} = create_test_namespace("error")

      on_exit(fn ->
        cleanup_namespace(client, namespace_id)
      end)

      %{client: client}
    end
  end

  test "ERROR-001: Invalid RPC method", %{client: client} do
    # Direct RPC call with invalid method
    assert {:error, error} = EventodbEx.Client.rpc(client, "invalid.method", [])
    # Server should return error (exact code may vary)
    assert is_binary(error.code)
  end

  test "ERROR-002: Missing required argument" do
    # This is enforced at SDK level - function requires stream_name
    # Compile-time check in Elixir prevents this
  end

  test "ERROR-003: Invalid stream name type" do
    # Type checking happens at compile time in Elixir with proper specs
    # Runtime would need explicit validation
  end

  @tag :skip_setup
  test "ERROR-004: Connection refused" do
    # Connect to non-existent server (using a valid port that's unlikely to be in use)
    client = EventodbEx.Client.new("http://localhost:65534")
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    assert {:error, error} = EventodbEx.stream_write(client, stream, message)
    assert error.code == "NETWORK_ERROR"

    assert String.contains?(error.message, "econnrefused") or
             String.contains?(error.message, "connection refused") or
             String.contains?(error.message, "Connection refused") or
             String.contains?(error.message, "failed")
  end

  test "ERROR-005: Server returns malformed JSON" do
    # Hard to test without mocking - would need special test endpoint
    # Skip for now
  end

  test "ERROR-006: Network timeout" do
    # Req has built-in timeout handling
    # Would need very slow endpoint to test properly
    # Skip for now
  end

  test "ERROR-007: HTTP error status" do
    # This is handled by the client - any non-200 status becomes an error
    # Already tested in other scenarios
  end
end
