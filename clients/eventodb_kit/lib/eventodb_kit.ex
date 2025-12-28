defmodule EventodbKit do
  @moduledoc """
  Production-ready Elixir SDK for EventoDB with resilience patterns.

  EventodbKit provides:
  - Outbox pattern for reliable event publishing
  - Consumer position tracking
  - Idempotency for producers and consumers
  - Singleton workers via Chosen
  - Built on top of EventodbEx
  """

  alias EventodbKit.Client
  alias EventodbKit.Outbox

  # Write operations (to outbox)

  @doc """
  Writes a message to the outbox for eventual delivery to EventoDB.

  ## Examples

      {:ok, outbox_id, kit} = EventodbKit.stream_write(
        kit,
        "account-123",
        %{type: "Deposited", data: %{amount: 100}}
      )
  """
  def stream_write(%Client{} = kit, stream, message, opts \\ []) do
    case Outbox.write(kit.repo, kit.namespace, stream, message, opts) do
      {:ok, outbox_id} -> {:ok, outbox_id, kit}
      {:error, reason} -> {:error, reason}
    end
  end

  @doc """
  Writes multiple messages to the outbox.
  """
  def stream_write_batch(%Client{} = kit, messages, opts \\ []) do
    case Outbox.write_batch(kit.repo, kit.namespace, messages, opts) do
      {:ok, outbox_ids} -> {:ok, outbox_ids, kit}
      {:error, reason} -> {:error, reason}
    end
  end

  # Read operations (delegate to EventodbEx)

  @doc """
  Reads all messages from a stream.
  """
  def stream_get(%Client{} = kit, stream, opts \\ %{}) do
    case EventodbEx.stream_get(kit.eventodb_client, stream, opts) do
      {:ok, messages, client} ->
        {:ok, messages, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Gets the last message from a stream.
  """
  def stream_last(%Client{} = kit, stream, opts \\ %{}) do
    case EventodbEx.stream_last(kit.eventodb_client, stream, opts) do
      {:ok, message, client} ->
        {:ok, message, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Gets the current version of a stream.
  """
  def stream_version(%Client{} = kit, stream) do
    case EventodbEx.stream_version(kit.eventodb_client, stream) do
      {:ok, version, client} ->
        {:ok, version, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Reads messages from a category.
  """
  def category_get(%Client{} = kit, category, opts \\ %{}) do
    case EventodbEx.category_get(kit.eventodb_client, category, opts) do
      {:ok, messages, client} ->
        {:ok, messages, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  # Namespace operations (delegate to EventodbEx)

  @doc """
  Creates a new namespace.
  """
  def namespace_create(%Client{} = kit, namespace_id, opts \\ %{}) do
    case EventodbEx.namespace_create(kit.eventodb_client, namespace_id, opts) do
      {:ok, result, client} ->
        {:ok, result, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Deletes a namespace.
  """
  def namespace_delete(%Client{} = kit, namespace_id) do
    case EventodbEx.namespace_delete(kit.eventodb_client, namespace_id) do
      {:ok, result, client} ->
        {:ok, result, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Lists all namespaces.
  """
  def namespace_list(%Client{} = kit) do
    case EventodbEx.namespace_list(kit.eventodb_client) do
      {:ok, list, client} ->
        {:ok, list, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc """
  Gets namespace information.
  """
  def namespace_info(%Client{} = kit, namespace_id) do
    case EventodbEx.namespace_info(kit.eventodb_client, namespace_id) do
      {:ok, info, client} ->
        {:ok, info, %{kit | eventodb_client: client}}

      {:error, reason} ->
        {:error, reason}
    end
  end
end
