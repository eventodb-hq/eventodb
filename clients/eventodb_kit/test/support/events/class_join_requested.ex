## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.ClassJoinRequested do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  Student requests to join a class using join code
  
  Category: class_membership
  Stream Pattern: class_membership-{class_id}+{student_id}
  """
  
  @primary_key false
  embedded_schema do
    field :class_id, :string
    field :student_id, :string
    field :join_code, :string
    field :requested_at, :utc_datetime
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:class_id, :student_id, :join_code, :requested_at])
    |> validate_required([:class_id, :student_id, :join_code, :requested_at])
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