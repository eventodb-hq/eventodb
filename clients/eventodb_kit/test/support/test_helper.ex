defmodule EventodbKit.TestHelper do
  @moduledoc """
  Test helper functions for EventodbKit tests.
  """

  @base_url System.get_env("EVENTODB_URL", "http://localhost:8080")
  @admin_token System.get_env("EVENTODB_ADMIN_TOKEN")

  def base_url, do: @base_url
  def admin_token, do: @admin_token

  @doc """
  Creates a test namespace and returns a kit client for it.
  """
  def create_test_namespace(test_name) do
    # Create namespace via EventodbEx
    admin_client = EventodbEx.Client.new(@base_url, token: @admin_token)
    namespace_id = "test-#{test_name}-#{unique_suffix()}"

    {:ok, result, _} =
      EventodbEx.namespace_create(admin_client, namespace_id, %{
        description: "Test namespace for #{test_name}"
      })

    # Create kit client
    kit =
      EventodbKit.Client.new(@base_url,
        token: result.token,
        repo: EventodbKit.TestRepo
      )

    {kit, namespace_id, result.token}
  end

  @doc """
  Cleans up a test namespace.
  """
  def cleanup_namespace(namespace_id) do
    admin_client = EventodbEx.Client.new(@base_url, token: @admin_token)
    EventodbEx.namespace_delete(admin_client, namespace_id)
  end

  @doc """
  Generates a unique suffix for test identifiers.
  """
  def unique_suffix do
    :erlang.unique_integer([:positive, :monotonic])
    |> Integer.to_string(36)
    |> String.downcase()
  end
end
