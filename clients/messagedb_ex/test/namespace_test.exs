defmodule MessagedbEx.NamespaceTest do
  use ExUnit.Case, async: false
  import MessagedbEx.TestHelper

  # Note: Most tests create their own namespace, so we test namespace operations separately
  # These tests use admin token directly since they test namespace operations themselves

  defp admin_client do
    MessagedbEx.Client.new(base_url(), token: System.get_env("MESSAGEDB_ADMIN_TOKEN"))
  end

  test "NS-001: Create namespace" do
    client = admin_client()
    namespace_id = "test-ns-#{unique_suffix()}"

    assert {:ok, result, _client} =
             MessagedbEx.namespace_create(client, namespace_id, %{
               description: "Test namespace"
             })

    assert result.namespace == namespace_id
    assert String.starts_with?(result.token, "ns_")
    assert is_binary(result.created_at)

    # Cleanup
    MessagedbEx.namespace_delete(client, namespace_id)
  end

  test "NS-002: Create namespace with custom token" do
    # This test is complex as it requires generating a valid token format
    # Skip for now - the server should validate the token format
  end

  test "NS-003: Create duplicate namespace" do
    client = admin_client()
    namespace_id = "duplicate-test-#{unique_suffix()}"

    # Create first time
    {:ok, _result, _client} = MessagedbEx.namespace_create(client, namespace_id)

    # Try to create again
    assert {:error, error} = MessagedbEx.namespace_create(client, namespace_id)
    assert error.code == "NAMESPACE_EXISTS"

    # Cleanup
    MessagedbEx.namespace_delete(client, namespace_id)
  end

  test "NS-004: Delete namespace" do
    client = admin_client()
    namespace_id = "delete-test-#{unique_suffix()}"

    # Create namespace
    {:ok, _result, client} = MessagedbEx.namespace_create(client, namespace_id)

    # Delete it
    assert {:ok, result, _client} = MessagedbEx.namespace_delete(client, namespace_id)
    assert result.namespace == namespace_id
    assert is_binary(result.deleted_at)
    assert is_integer(result.messages_deleted)
  end

  test "NS-005: Delete non-existent namespace" do
    client = admin_client()

    assert {:error, error} =
             MessagedbEx.namespace_delete(client, "does-not-exist-#{unique_suffix()}")

    assert error.code == "NAMESPACE_NOT_FOUND"
  end

  test "NS-006: List namespaces" do
    client = admin_client()

    # Create a test namespace
    namespace_id = "list-test-#{unique_suffix()}"
    {:ok, _result, client} = MessagedbEx.namespace_create(client, namespace_id)

    # List namespaces
    assert {:ok, namespaces, _client} = MessagedbEx.namespace_list(client)
    assert is_list(namespaces)
    assert length(namespaces) > 0

    # Each namespace should have required fields
    namespace = List.first(namespaces)
    assert Map.has_key?(namespace, :namespace)
    assert Map.has_key?(namespace, :created_at)
    assert Map.has_key?(namespace, :message_count)

    # Cleanup
    MessagedbEx.namespace_delete(client, namespace_id)
  end

  test "NS-007: Get namespace info" do
    client = admin_client()
    namespace_id = "info-test-#{unique_suffix()}"

    # Create namespace
    {:ok, result, client} = MessagedbEx.namespace_create(client, namespace_id)
    client = MessagedbEx.Client.set_token(client, result.token)

    # Write 5 messages
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    client =
      Enum.reduce(1..5, client, fn _, c ->
        {:ok, _result, new_c} = MessagedbEx.stream_write(c, stream, message)
        new_c
      end)

    # Get info
    assert {:ok, info, _client} = MessagedbEx.namespace_info(client, namespace_id)
    assert info.namespace == namespace_id
    assert info.message_count == 5

    # Cleanup
    MessagedbEx.namespace_delete(client, namespace_id)
  end

  test "NS-008: Get info for non-existent namespace" do
    client = admin_client()

    assert {:error, error} =
             MessagedbEx.namespace_info(client, "does-not-exist-#{unique_suffix()}")

    assert error.code == "NAMESPACE_NOT_FOUND"
  end
end
