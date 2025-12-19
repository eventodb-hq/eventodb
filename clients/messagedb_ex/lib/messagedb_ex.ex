defmodule MessagedbEx do
  @moduledoc """
  Elixir client for MessageDB - a simple, fast message store.

  ## Usage

      # Create client
      client = MessagedbEx.Client.new("http://localhost:8080", token: "ns_...")

      # Write message
      {:ok, result, client} = MessagedbEx.stream_write(
        client,
        "account-123",
        %{type: "Deposited", data: %{amount: 100}}
      )

      # Read stream
      {:ok, messages, client} = MessagedbEx.stream_get(client, "account-123")

  """

  alias MessagedbEx.{Client, Error, Types}

  # ====================
  # Stream Operations
  # ====================

  @doc """
  Writes a message to a stream.

  ## Options

    * `:id` - Custom message UUID (auto-generated if omitted)
    * `:expected_version` - Expected stream version for optimistic locking

  ## Examples

      {:ok, result, client} = MessagedbEx.stream_write(
        client,
        "account-123",
        %{type: "Deposited", data: %{amount: 100}},
        %{expected_version: 0}
      )

  """
  @spec stream_write(Client.t(), String.t(), Types.message(), map()) ::
          {:ok, Types.write_result(), Client.t()} | {:error, Error.t()}
  def stream_write(client, stream_name, message, opts \\ %{}) do
    opts = normalize_options(opts)
    with {:ok, result, client} <- Client.rpc(client, "stream.write", [stream_name, message, opts]) do
      {:ok, snake_case_keys(result), client}
    end
  end

  @doc """
  Reads messages from a stream.

  ## Options

    * `:position` - Starting position (inclusive, default: 0)
    * `:global_position` - Alternative: filter by global position
    * `:batch_size` - Max messages to return (default: 1000, -1 for unlimited, max 10000)

  ## Examples

      {:ok, messages, client} = MessagedbEx.stream_get(
        client,
        "account-123",
        %{position: 0, batch_size: 10}
      )

  """
  @spec stream_get(Client.t(), String.t(), map()) ::
          {:ok, list(Types.stream_message()), Client.t()} | {:error, Error.t()}
  def stream_get(client, stream_name, opts \\ %{}) do
    opts = normalize_options(opts)
    Client.rpc(client, "stream.get", [stream_name, opts])
  end

  @doc """
  Gets the last message from a stream.

  ## Options

    * `:type` - Filter by event type

  Returns `nil` if stream is empty or doesn't exist.

  ## Examples

      {:ok, message, client} = MessagedbEx.stream_last(client, "account-123")
      {:ok, message, client} = MessagedbEx.stream_last(client, "account-123", %{type: "Deposited"})

  """
  @spec stream_last(Client.t(), String.t(), map()) ::
          {:ok, Types.stream_message() | nil, Client.t()} | {:error, Error.t()}
  def stream_last(client, stream_name, opts \\ %{}) do
    Client.rpc(client, "stream.last", [stream_name, opts])
  end

  @doc """
  Gets the current version (latest position) of a stream.

  Returns `nil` if stream doesn't exist.

  Note: Version is 0-based, so version 5 means 6 messages (positions 0-5).

  ## Examples

      {:ok, version, client} = MessagedbEx.stream_version(client, "account-123")

  """
  @spec stream_version(Client.t(), String.t()) ::
          {:ok, integer() | nil, Client.t()} | {:error, Error.t()}
  def stream_version(client, stream_name) do
    Client.rpc(client, "stream.version", [stream_name])
  end

  # ====================
  # Category Operations
  # ====================

  @doc """
  Reads messages from all streams in a category.

  ## Options

    * `:position` - Starting global position (default: 0)
    * `:global_position` - Alternative to position
    * `:batch_size` - Max messages to return (default: 1000)
    * `:correlation` - Filter by correlationStreamName category
    * `:consumer_group` - Map with `:member` (0-based index) and `:size` (total consumers)

  ## Examples

      {:ok, messages, client} = MessagedbEx.category_get(client, "account")

      {:ok, messages, client} = MessagedbEx.category_get(
        client,
        "account",
        %{
          batch_size: 100,
          consumer_group: %{member: 0, size: 4}
        }
      )

  """
  @spec category_get(Client.t(), String.t(), map()) ::
          {:ok, list(Types.category_message()), Client.t()} | {:error, Error.t()}
  def category_get(client, category_name, opts \\ %{}) do
    opts = normalize_options(opts)
    Client.rpc(client, "category.get", [category_name, opts])
  end

  # =======================
  # Namespace Operations
  # =======================

  @doc """
  Creates a new namespace.

  ## Options

    * `:description` - Human-readable description
    * `:token` - Custom token (must be valid format for namespace)

  ## Examples

      {:ok, result, client} = MessagedbEx.namespace_create(
        client,
        "my-namespace",
        %{description: "My application namespace"}
      )

  """
  @spec namespace_create(Client.t(), String.t(), map()) ::
          {:ok, map(), Client.t()} | {:error, Error.t()}
  def namespace_create(client, namespace_id, opts \\ %{}) do
    with {:ok, result, client} <- Client.rpc(client, "ns.create", [namespace_id, opts]) do
      {:ok, snake_case_keys(result), client}
    end
  end

  @doc """
  Deletes a namespace and all its data.

  ⚠️ Warning: This operation is irreversible.

  ## Examples

      {:ok, result, client} = MessagedbEx.namespace_delete(client, "my-namespace")

  """
  @spec namespace_delete(Client.t(), String.t()) ::
          {:ok, map(), Client.t()} | {:error, Error.t()}
  def namespace_delete(client, namespace_id) do
    with {:ok, result, client} <- Client.rpc(client, "ns.delete", [namespace_id]) do
      {:ok, snake_case_keys(result), client}
    end
  end

  @doc """
  Lists all namespaces.

  ## Examples

      {:ok, namespaces, client} = MessagedbEx.namespace_list(client)

  """
  @spec namespace_list(Client.t()) ::
          {:ok, list(map()), Client.t()} | {:error, Error.t()}
  def namespace_list(client) do
    with {:ok, result, client} <- Client.rpc(client, "ns.list", []) do
      {:ok, Enum.map(result, &snake_case_keys/1), client}
    end
  end

  @doc """
  Gets detailed information about a namespace.

  ## Examples

      {:ok, info, client} = MessagedbEx.namespace_info(client, "my-namespace")

  """
  @spec namespace_info(Client.t(), String.t()) ::
          {:ok, map(), Client.t()} | {:error, Error.t()}
  def namespace_info(client, namespace_id) do
    with {:ok, result, client} <- Client.rpc(client, "ns.info", [namespace_id]) do
      {:ok, snake_case_keys(result), client}
    end
  end

  # ===================
  # System Operations
  # ===================

  @doc """
  Gets the server version.

  ## Examples

      {:ok, version, client} = MessagedbEx.system_version(client)

  """
  @spec system_version(Client.t()) ::
          {:ok, String.t(), Client.t()} | {:error, Error.t()}
  def system_version(client) do
    Client.rpc(client, "sys.version", [])
  end

  @doc """
  Gets server health status.

  ## Examples

      {:ok, health, client} = MessagedbEx.system_health(client)

  """
  @spec system_health(Client.t()) ::
          {:ok, map(), Client.t()} | {:error, Error.t()}
  def system_health(client) do
    with {:ok, result, client} <- Client.rpc(client, "sys.health", []) do
      {:ok, snake_case_keys(result), client}
    end
  end

  # Private helpers

  defp snake_case_keys(map) when is_map(map) do
    Map.new(map, fn {k, v} ->
      snake_key = k
        |> Macro.underscore()
        |> String.to_atom()
      {snake_key, v}
    end)
  end

  # Convert Elixir-style snake_case options to API camelCase
  defp normalize_options(opts) when is_map(opts) do
    opts
    |> Enum.map(fn {k, v} -> {to_camel_case(k), normalize_value(v)} end)
    |> Map.new()
  end

  defp normalize_value(%{member: member, size: size}) do
    # Consumer group
    %{"member" => member, "size" => size}
  end

  defp normalize_value(v), do: v

  defp to_camel_case(atom) when is_atom(atom) do
    atom |> Atom.to_string() |> to_camel_case()
  end

  defp to_camel_case("batch_size"), do: "batchSize"
  defp to_camel_case("global_position"), do: "globalPosition"
  defp to_camel_case("consumer_group"), do: "consumerGroup"
  defp to_camel_case("expected_version"), do: "expectedVersion"
  defp to_camel_case(s) when is_binary(s), do: s
end
