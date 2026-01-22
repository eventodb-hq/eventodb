defmodule EventodbKit.Consumer.Position do
  @moduledoc """
  Position tracking for consumers.
  """

  import Ecto.Query
  alias EventodbKit.Schema.ConsumerPosition

  @doc """
  Loads the current position for a consumer.
  Returns nil if no position is found (fresh consumer).
  """
  def load(repo, namespace, category, consumer_id) do
    query =
      from(p in ConsumerPosition,
        where:
          p.namespace == ^namespace and
            p.category == ^category and
            p.consumer_id == ^consumer_id
      )

    case repo.one(query) do
      nil -> nil
      position -> position.position
    end
  end

  @doc """
  Saves the current position for a consumer.
  """
  def save(repo, namespace, category, consumer_id, position) do
    attrs = %{
      namespace: namespace,
      category: category,
      consumer_id: consumer_id,
      position: position,
      updated_at: DateTime.utc_now()
    }

    %ConsumerPosition{}
    |> ConsumerPosition.changeset(attrs)
    |> repo.insert(
      on_conflict: {:replace, [:position, :updated_at]},
      conflict_target: [:namespace, :category, :consumer_id]
    )
  end
end
