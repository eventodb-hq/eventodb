defmodule EventodbKit.Schema.ProcessedEvent do
  @moduledoc """
  Schema for the evento_processed_events table.
  """
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key {:event_id, :binary_id, autogenerate: false}
  schema "evento_processed_events" do
    field(:namespace, :string)
    field(:event_type, :string)
    field(:category, :string)
    field(:consumer_id, :string)
    field(:processed_at, :utc_datetime)
  end

  def changeset(processed_event, attrs) do
    processed_event
    |> cast(attrs, [:event_id, :namespace, :event_type, :category, :consumer_id, :processed_at])
    |> validate_required([:event_id, :namespace, :event_type, :category, :consumer_id])
  end
end
