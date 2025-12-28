defmodule EventodbKit.Schema.ConsumerPosition do
  @moduledoc """
  Schema for the evento_consumer_positions table.
  """
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  schema "evento_consumer_positions" do
    field(:namespace, :string, primary_key: true)
    field(:category, :string, primary_key: true)
    field(:consumer_id, :string, primary_key: true)
    field(:position, :integer)
    field(:updated_at, :utc_datetime)
  end

  def changeset(position, attrs) do
    position
    |> cast(attrs, [:namespace, :category, :consumer_id, :position, :updated_at])
    |> validate_required([:namespace, :category, :consumer_id, :position])
  end
end
