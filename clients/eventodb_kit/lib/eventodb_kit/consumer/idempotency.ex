defmodule EventodbKit.Consumer.Idempotency do
  @moduledoc """
  Idempotency tracking for consumers to prevent duplicate processing.
  """

  import Ecto.Query
  alias EventodbKit.Schema.ProcessedEvent

  @doc """
  Checks if an event has already been processed.
  """
  def processed?(repo, event_id) do
    query = from(p in ProcessedEvent, where: p.event_id == ^event_id)
    repo.exists?(query)
  end

  @doc """
  Marks an event as processed.
  """
  def mark_processed(repo, event_id, namespace, event_type, category, consumer_id) do
    attrs = %{
      event_id: event_id,
      namespace: namespace,
      event_type: event_type,
      category: category,
      consumer_id: consumer_id,
      processed_at: DateTime.utc_now()
    }

    %ProcessedEvent{}
    |> ProcessedEvent.changeset(attrs)
    |> repo.insert(on_conflict: :nothing)
  end

  @doc """
  Cleans up old processed events.
  """
  def cleanup(repo, namespace, before_datetime) do
    from(p in ProcessedEvent,
      where: p.namespace == ^namespace and p.processed_at < ^before_datetime
    )
    |> repo.delete_all()
  end
end
