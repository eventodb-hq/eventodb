defmodule EventodbKit.Outbox do
  @moduledoc """
  Outbox pattern implementation for EventodbKit.
  Writes events to local database before sending to EventoDB.
  """

  import Ecto.Query
  alias EventodbKit.Schema.Outbox

  @doc """
  Writes a message to the outbox.

  Returns `{:ok, outbox_id}` or `{:error, reason}`.

  ## Options

  - `:expected_version` - Expected stream version for optimistic locking
  - `:id` - Custom message ID
  - `:metadata` - Custom metadata map
  """
  def write(repo, namespace, stream, message, opts \\ []) do
    idempotency_key = get_in(message, [:data, :idempotency_key])

    if idempotency_key do
      write_idempotent(repo, namespace, stream, message, opts, idempotency_key)
    else
      write_new(repo, namespace, stream, message, opts)
    end
  end

  @doc """
  Writes multiple messages to the outbox in a batch.
  """
  def write_batch(repo, namespace, messages, opts \\ []) do
    multi =
      Enum.reduce(messages, Ecto.Multi.new(), fn {stream, message}, multi ->
        key = "#{stream}-#{System.unique_integer([:positive])}"
        Ecto.Multi.run(multi, key, fn repo, _changes ->
          case write(repo, namespace, stream, message, opts) do
            {:ok, id} -> {:ok, id}
            {:error, reason} -> {:error, reason}
          end
        end)
      end)

    case repo.transaction(multi) do
      {:ok, results} ->
        ids = Enum.map(results, fn {_key, id} -> id end)
        {:ok, ids}

      {:error, _failed_op, reason, _changes} ->
        {:error, reason}
    end
  end

  @doc """
  Fetches unsent messages from the outbox.
  """
  def fetch_unsent(repo, namespace, opts \\ []) do
    batch_size = Keyword.get(opts, :batch_size, 100)

    query =
      from(o in Outbox,
        where: o.namespace == ^namespace and is_nil(o.sent_at),
        order_by: [asc: o.created_at],
        limit: ^batch_size
      )

    repo.all(query)
  end

  @doc """
  Marks a message as sent.
  """
  def mark_sent(repo, outbox_id) do
    repo.get!(Outbox, outbox_id)
    |> Ecto.Changeset.change(sent_at: DateTime.utc_now())
    |> repo.update()
  end

  @doc """
  Marks multiple messages as sent.
  """
  def mark_sent_batch(repo, outbox_ids) do
    from(o in Outbox,
      where: o.id in ^outbox_ids
    )
    |> repo.update_all(set: [sent_at: DateTime.utc_now()])
  end

  # Private functions

  defp write_new(repo, namespace, stream, message, opts) do
    attrs = %{
      namespace: namespace,
      stream: stream,
      type: message[:type] || message["type"],
      data: message[:data] || message["data"],
      metadata: message[:metadata] || message["metadata"],
      write_options: build_write_options(opts)
    }

    %Outbox{}
    |> Outbox.changeset(attrs)
    |> repo.insert()
    |> case do
      {:ok, outbox} -> {:ok, outbox.id}
      {:error, changeset} -> {:error, changeset}
    end
  end

  defp write_idempotent(repo, namespace, stream, message, opts, idempotency_key) do
    # Check if already exists
    query =
      from(o in Outbox,
        where: fragment("?->>'idempotency_key' = ?", o.data, ^idempotency_key),
        limit: 1
      )

    case repo.one(query) do
      nil -> write_new(repo, namespace, stream, message, opts)
      existing -> {:ok, existing.id}
    end
  end

  defp build_write_options(opts) do
    opts
    |> Enum.filter(fn {key, _value} ->
      key in [:expected_version, :id]
    end)
    |> Enum.into(%{})
  end
end
