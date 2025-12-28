defmodule EventodbKit.Schema.Outbox do
  @moduledoc """
  Schema for the evento_outbox table.
  """
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key {:id, :binary_id, autogenerate: true}
  schema "evento_outbox" do
    field(:namespace, :string)
    field(:stream, :string)
    field(:type, :string)
    field(:data, :map)
    field(:metadata, :map)
    field(:write_options, :map)
    field(:sent_at, :utc_datetime_usec)
    field(:created_at, :utc_datetime_usec)
  end

  def changeset(outbox, attrs) do
    outbox
    |> cast(attrs, [:namespace, :stream, :type, :data, :metadata, :write_options, :sent_at])
    |> validate_required([:namespace, :stream, :type, :data])
  end
end
