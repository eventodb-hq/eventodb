## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.StudentProgressMilestoneReached do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  Student reaches a significant progress milestone
  
  Category: student_progress
  Stream Pattern: student_progress-{student_id}
  """
  
  @primary_key false
  embedded_schema do
    field :student_id, :string
    field :milestone_type, :string
    field :milestone_value, :string
    field :achieved_at, :utc_datetime
    field :related_course_id, :string
    embeds_one :metadata, Metadata, primary_key: false do
      field :score, :integer
      field :time_spent, :integer
      field :attempts, :integer
    end
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:student_id, :milestone_type, :milestone_value, :achieved_at, :related_course_id])
    |> validate_required([:student_id, :milestone_type, :milestone_value, :achieved_at])
    |> validate_inclusion(:milestone_type, ["level_completed", "course_completed", "streak_achieved", "mastery_reached"])
    |> cast_embed(:metadata)
  end
  
  @doc "Returns the event category"
  def category, do: "student_progress"
  
  @doc "Builds stream name from data"
  def stream_name(data) when is_map(data) do
    "student_progress-#{data.student_id}"
  end
  
  @doc "Validates data and returns result"
  def validate!(data) do
    case changeset(data) do
      %{valid?: true} = changeset -> {:ok, apply_changes(changeset)}
      changeset -> {:error, changeset}
    end
  end
end