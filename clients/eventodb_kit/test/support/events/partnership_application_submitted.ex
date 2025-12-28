## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.PartnershipApplicationSubmitted do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  School submits partnership application form on website
  
  Category: partnership_application
  Stream Pattern: partnership_application-{application_id}
  """
  
  @primary_key false
  embedded_schema do
    field :application_id, :binary_id
    field :school_name, :string
    field :contact_name, :string
    field :contact_email, :string
    field :contact_phone, :string
    field :submitted_at, :utc_datetime
    embeds_one :metadata, Metadata, primary_key: false do
      field :source, :string
      field :ip_address, :string
      field :user_agent, :string
    end
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:application_id, :school_name, :contact_name, :contact_email, :contact_phone, :submitted_at])
    |> validate_required([:application_id, :school_name, :contact_name, :contact_email, :contact_phone, :submitted_at])
    |> validate_format(:application_id, ~r/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/)
    |> validate_format(:contact_email, ~r/^[^\s@]+@[^\s@]+\.[^\s@]+$/)
    |> cast_embed(:metadata)
  end
  
  @doc "Returns the event category"
  def category, do: "partnership_application"
  
  @doc "Builds stream name from data"
  def stream_name(data) when is_map(data) do
    "partnership_application-#{data.application_id}"
  end
  
  @doc "Validates data and returns result"
  def validate!(data) do
    case changeset(data) do
      %{valid?: true} = changeset -> {:ok, apply_changes(changeset)}
      changeset -> {:error, changeset}
    end
  end
end