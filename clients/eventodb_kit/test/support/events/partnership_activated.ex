## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.PartnershipActivated do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  CRM admin activates a partnership after review
  
  Category: partnership
  Stream Pattern: partnership-{school_id}
  """
  
  @primary_key false
  embedded_schema do
    field :school_id, :string
    field :application_id, :binary_id
    field :claim_code, :string
    field :activated_at, :utc_datetime
    field :tier, :string
    field :expires_at, :utc_datetime
    field :activated_by, :string
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:school_id, :application_id, :claim_code, :activated_at, :tier, :expires_at, :activated_by])
    |> validate_required([:school_id, :application_id, :claim_code, :activated_at, :tier, :activated_by])
    |> validate_format(:application_id, ~r/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/)
    |> validate_inclusion(:tier, ["trial", "basic", "premium"])
  end
  
  @doc "Returns the event category"
  def category, do: "partnership"
  
  @doc "Builds stream name from data"
  def stream_name(data) when is_map(data) do
    "partnership-#{data.school_id}"
  end
  
  @doc "Validates data and returns result"
  def validate!(data) do
    case changeset(data) do
      %{valid?: true} = changeset -> {:ok, apply_changes(changeset)}
      changeset -> {:error, changeset}
    end
  end
end