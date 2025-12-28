## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.ClassMembershipAccepted do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  Teacher approves student join request
  
  Category: class_membership
  Stream Pattern: class_membership-{class_id}+{student_id}
  """
  
  @primary_key false
  embedded_schema do
    field :class_id, :string
    field :student_id, :string
    field :accepted_at, :utc_datetime
    field :accepted_by, :string
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:class_id, :student_id, :accepted_at, :accepted_by])
    |> validate_required([:class_id, :student_id, :accepted_at, :accepted_by])
  end
  
  @doc "Returns the event category"
  def category, do: "class_membership"
  
  @doc "Builds stream name from data"
  def stream_name(data) when is_map(data) do
    "class_membership-#{data.class_id}+#{data.student_id}"
  end
  
  @doc "Validates data and returns result"
  def validate!(data) do
    case changeset(data) do
      %{valid?: true} = changeset -> {:ok, apply_changes(changeset)}
      changeset -> {:error, changeset}
    end
  end
end