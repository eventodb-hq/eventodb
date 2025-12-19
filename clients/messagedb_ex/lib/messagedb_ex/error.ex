defmodule MessagedbEx.Error do
  @moduledoc """
  Exception type for MessageDB errors.
  """

  defexception [:code, :message, :details]

  @type t :: %__MODULE__{
          code: String.t(),
          message: String.t(),
          details: map() | nil
        }

  @doc """
  Creates an error from an HTTP response error body.
  """
  @spec from_response(map()) :: t()
  def from_response(%{"error" => error}) do
    %__MODULE__{
      code: error["code"],
      message: error["message"] || "Unknown error",
      details: error["details"]
    }
  end

  def from_response(error) when is_binary(error) do
    %__MODULE__{
      code: "UNKNOWN_ERROR",
      message: error,
      details: nil
    }
  end

  @doc """
  Creates a network error.
  """
  @spec network_error(String.t()) :: t()
  def network_error(message) do
    %__MODULE__{
      code: "NETWORK_ERROR",
      message: message,
      details: nil
    }
  end
end
