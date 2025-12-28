## **** GENERATED CODE! see ElixirGenerator for details. ****

defmodule Events.ContentErrorReported do
  use Ecto.Schema
  import Ecto.Changeset
  
  @moduledoc """
  Student or teacher reports an error in course content
  
  Category: content_error
  Stream Pattern: content_error-{error_id}
  """
  
  @primary_key false
  embedded_schema do
    field :error_id, :binary_id
    field :content_id, :string
    field :content_type, :string
    field :error_type, :string
    field :description, :string
    field :reported_by, :string
    field :reported_at, :utc_datetime
    embeds_one :context, Context, primary_key: false do
      field :url, :string
      field :user_answer, :string
      field :screenshot, :string
    end
  end
  
  @doc "Changeset for validation"
  def changeset(data \\ %{}) do
    %__MODULE__{}
    |> cast(data, [:error_id, :content_id, :content_type, :error_type, :description, :reported_by, :reported_at])
    |> validate_required([:error_id, :content_id, :content_type, :error_type, :description, :reported_at])
    |> validate_format(:error_id, ~r/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/)
    |> validate_inclusion(:content_type, ["question", "lesson", "exercise", "video"])
    |> validate_inclusion(:error_type, ["typo", "wrong_answer", "broken_media", "unclear", "other"])
    |> cast_embed(:context)
  end
  
  @doc "Returns the event category"
  def category, do: "content_error"
  
  @doc "Builds stream name from data"
  def stream_name(data) when is_map(data) do
    "content_error-#{data.error_id}"
  end
  
  @doc "Validates data and returns result"
  def validate!(data) do
    case changeset(data) do
      %{valid?: true} = changeset -> {:ok, apply_changes(changeset)}
      changeset -> {:error, changeset}
    end
  end
end