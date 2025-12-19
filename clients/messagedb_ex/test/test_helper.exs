ExUnit.start()

defmodule MessagedbEx.TestHelper do
  @moduledoc """
  Helper functions for MessageDB tests.
  """

  @base_url System.get_env("MESSAGEDB_URL", "http://localhost:8080")
  @admin_token System.get_env("MESSAGEDB_ADMIN_TOKEN")

  @doc """
  Creates a test namespace for isolation.
  Returns {client, namespace_id, token}.
  
  Uses MESSAGEDB_ADMIN_TOKEN env var to authenticate namespace creation.
  """
  def create_test_namespace(test_name) do
    # Create admin client for namespace creation
    admin_client = MessagedbEx.Client.new(@base_url, token: @admin_token)
    namespace_id = "test-#{test_name}-#{unique_suffix()}"

    {:ok, result, _admin_client} =
      MessagedbEx.namespace_create(admin_client, namespace_id, %{
        description: "Test namespace for #{test_name}"
      })

    # Return a NEW client with the new namespace's token
    client = MessagedbEx.Client.new(@base_url, token: result.token)
    {client, namespace_id, result.token}
  end

  @doc """
  Cleans up a test namespace.
  Uses admin token to delete.
  """
  def cleanup_namespace(_client, namespace_id) do
    admin_client = MessagedbEx.Client.new(@base_url, token: @admin_token)
    MessagedbEx.namespace_delete(admin_client, namespace_id)
  end

  @doc """
  Generates a unique stream name.
  """
  def unique_stream(prefix \\ "test") do
    "#{prefix}-#{unique_suffix()}"
  end

  @doc """
  Generates a unique suffix for test isolation.
  """
  def unique_suffix do
    :erlang.unique_integer([:positive, :monotonic])
    |> Integer.to_string(36)
    |> String.downcase()
  end

  @doc """
  Gets the base URL for tests.
  """
  def base_url, do: @base_url
end
