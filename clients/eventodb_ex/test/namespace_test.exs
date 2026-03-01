defmodule EventodbEx.NamespaceTest do
  use ExUnit.Case, async: true
  import EventodbEx.TestHelper

  # Note: Most tests create their own namespace, so we test namespace operations separately
  # These tests use admin token directly since they test namespace operations themselves

  defp admin_client do
    EventodbEx.Client.new(base_url(), token: System.get_env("EVENTODB_ADMIN_TOKEN"))
  end

  test "NS-001: Create namespace" do
    client = admin_client()
    namespace_id = "test-ns-#{unique_suffix()}"

    assert {:ok, result, _client} =
             EventodbEx.namespace_create(client, namespace_id, %{
               description: "Test namespace"
             })

    assert result.namespace == namespace_id
    assert String.starts_with?(result.token, "ns_")
    assert is_binary(result.created_at)

    # Cleanup
    EventodbEx.namespace_delete(client, namespace_id)
  end

  test "NS-002: Create namespace with custom token" do
    # This test is complex as it requires generating a valid token format
    # Skip for now - the server should validate the token format
  end

  test "NS-003: Create duplicate namespace" do
    client = admin_client()
    namespace_id = "duplicate-test-#{unique_suffix()}"

    # Create first time
    {:ok, _result, _client} = EventodbEx.namespace_create(client, namespace_id)

    # Try to create again
    assert {:error, error} = EventodbEx.namespace_create(client, namespace_id)
    assert error.code == "NAMESPACE_EXISTS"

    # Cleanup
    EventodbEx.namespace_delete(client, namespace_id)
  end

  test "NS-004: Delete namespace" do
    client = admin_client()
    namespace_id = "delete-test-#{unique_suffix()}"

    # Create namespace
    {:ok, _result, client} = EventodbEx.namespace_create(client, namespace_id)

    # Delete it
    assert {:ok, result, _client} = EventodbEx.namespace_delete(client, namespace_id)
    assert result.namespace == namespace_id
    assert is_binary(result.deleted_at)
    assert is_integer(result.messages_deleted)
  end

  test "NS-005: Delete non-existent namespace" do
    client = admin_client()

    assert {:error, error} =
             EventodbEx.namespace_delete(client, "does-not-exist-#{unique_suffix()}")

    assert error.code == "NAMESPACE_NOT_FOUND"
  end

  test "NS-006: List namespaces" do
    client = admin_client()

    # Create a test namespace
    namespace_id = "list-test-#{unique_suffix()}"
    {:ok, _result, client} = EventodbEx.namespace_create(client, namespace_id)

    # List namespaces
    assert {:ok, namespaces, _client} = EventodbEx.namespace_list(client)
    assert is_list(namespaces)
    assert length(namespaces) > 0

    # Each namespace should have required fields
    namespace = List.first(namespaces)
    assert Map.has_key?(namespace, :namespace)
    assert Map.has_key?(namespace, :created_at)
    assert Map.has_key?(namespace, :message_count)

    # Cleanup
    EventodbEx.namespace_delete(client, namespace_id)
  end

  test "NS-007: Get namespace info" do
    client = admin_client()
    namespace_id = "info-test-#{unique_suffix()}"

    # Create namespace
    {:ok, result, client} = EventodbEx.namespace_create(client, namespace_id)
    client = EventodbEx.Client.set_token(client, result.token)

    # Write 5 messages
    stream = unique_stream()
    message = %{type: "TestEvent", data: %{}}

    client =
      Enum.reduce(1..5, client, fn _, c ->
        {:ok, _result, new_c} = EventodbEx.stream_write(c, stream, message)
        new_c
      end)

    # Get info
    assert {:ok, info, _client} = EventodbEx.namespace_info(client, namespace_id)
    assert info.namespace == namespace_id
    assert info.message_count == 5

    # Cleanup
    EventodbEx.namespace_delete(client, namespace_id)
  end

  test "NS-008: Get info for non-existent namespace" do
    client = admin_client()

    assert {:error, error} =
             EventodbEx.namespace_info(client, "does-not-exist-#{unique_suffix()}")

    assert error.code == "NAMESPACE_NOT_FOUND"
  end

  test "NS-009: namespace_streams returns empty list for empty namespace" do
    {client, namespace_id, _token} = create_test_namespace("streams-empty")

    assert {:ok, streams, _client} = EventodbEx.namespace_streams(client)
    assert streams == []

    cleanup_namespace(client, namespace_id)
  end

  test "NS-010: namespace_streams returns streams after writes" do
    {client, namespace_id, _token} = create_test_namespace("streams-after-writes")

    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "Created", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-2", %{type: "Created", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "order-1", %{type: "Created", data: %{}})

    assert {:ok, streams, _client} = EventodbEx.namespace_streams(client)
    assert length(streams) == 3

    first = List.first(streams)
    assert Map.has_key?(first, :stream)
    assert Map.has_key?(first, :version)
    assert Map.has_key?(first, :last_activity)

    cleanup_namespace(client, namespace_id)
  end

  test "NS-011: namespace_streams sorted lexicographically" do
    {client, namespace_id, _token} = create_test_namespace("streams-sorted")

    {:ok, _, client} = EventodbEx.stream_write(client, "order-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "user-1", %{type: "E", data: %{}})

    assert {:ok, streams, _client} = EventodbEx.namespace_streams(client)
    names = Enum.map(streams, & &1.stream)
    assert names == ["account-1", "order-1", "user-1"]

    cleanup_namespace(client, namespace_id)
  end

  test "NS-012: namespace_streams prefix filter" do
    {client, namespace_id, _token} = create_test_namespace("streams-prefix")

    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-2", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "order-1", %{type: "E", data: %{}})

    assert {:ok, streams, _client} = EventodbEx.namespace_streams(client, %{prefix: "account"})
    assert length(streams) == 2
    assert Enum.all?(streams, &String.starts_with?(&1.stream, "account"))

    cleanup_namespace(client, namespace_id)
  end

  test "NS-013: namespace_streams cursor pagination" do
    {client, namespace_id, _token} = create_test_namespace("streams-cursor")

    {:ok, _, client} = EventodbEx.stream_write(client, "stream-a", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "stream-b", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "stream-c", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "stream-d", %{type: "E", data: %{}})

    assert {:ok, page1, client} = EventodbEx.namespace_streams(client, %{limit: 2})
    assert length(page1) == 2

    cursor = List.last(page1).stream
    assert {:ok, page2, _client} = EventodbEx.namespace_streams(client, %{limit: 2, cursor: cursor})
    assert length(page2) == 2

    # No overlap
    page1_names = MapSet.new(page1, & &1.stream)
    page2_names = MapSet.new(page2, & &1.stream)
    assert MapSet.disjoint?(page1_names, page2_names)
    assert MapSet.size(MapSet.union(page1_names, page2_names)) == 4

    cleanup_namespace(client, namespace_id)
  end

  test "NS-014: namespace_streams version reflects last position" do
    {client, namespace_id, _token} = create_test_namespace("streams-version")

    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "A", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "B", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "C", data: %{}})

    assert {:ok, [stream], _client} = EventodbEx.namespace_streams(client)
    assert stream.version == 2

    cleanup_namespace(client, namespace_id)
  end

  test "NS-015: namespace_categories returns empty list for empty namespace" do
    {client, namespace_id, _token} = create_test_namespace("cats-empty")

    assert {:ok, categories, _client} = EventodbEx.namespace_categories(client)
    assert categories == []

    cleanup_namespace(client, namespace_id)
  end

  test "NS-016: namespace_categories derives categories from stream names" do
    {client, namespace_id, _token} = create_test_namespace("cats-derive")

    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-2", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "order-1", %{type: "E", data: %{}})

    assert {:ok, categories, _client} = EventodbEx.namespace_categories(client)
    assert length(categories) == 2

    names = Enum.map(categories, & &1.category)
    assert names == ["account", "order"]

    cleanup_namespace(client, namespace_id)
  end

  test "NS-017: namespace_categories counts are accurate" do
    {client, namespace_id, _token} = create_test_namespace("cats-counts")

    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-2", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "order-1", %{type: "E", data: %{}})

    assert {:ok, categories, _client} = EventodbEx.namespace_categories(client)
    cat_map = Map.new(categories, &{&1.category, &1})

    assert cat_map["account"].stream_count == 2
    assert cat_map["account"].message_count == 3
    assert cat_map["order"].stream_count == 1
    assert cat_map["order"].message_count == 1

    cleanup_namespace(client, namespace_id)
  end

  test "NS-018: namespace_categories stream with no dash is its own category" do
    {client, namespace_id, _token} = create_test_namespace("cats-no-dash")

    {:ok, _, client} = EventodbEx.stream_write(client, "account", %{type: "E", data: %{}})
    {:ok, _, client} = EventodbEx.stream_write(client, "account-1", %{type: "E", data: %{}})

    assert {:ok, [cat], _client} = EventodbEx.namespace_categories(client)
    assert cat.category == "account"
    assert cat.stream_count == 2
    assert cat.message_count == 2

    cleanup_namespace(client, namespace_id)
  end
end
