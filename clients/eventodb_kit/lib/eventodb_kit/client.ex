defmodule EventodbKit.Client do
  @moduledoc """
  EventodbKit client that wraps EventodbEx.Client with repository support.
  """

  defstruct [:eventodb_client, :repo, :namespace]

  @type t :: %__MODULE__{
          eventodb_client: EventodbEx.Client.t(),
          repo: module(),
          namespace: String.t() | nil
        }

  @doc """
  Creates a new EventodbKit client.

  ## Examples

      kit = EventodbKit.Client.new(
        "http://localhost:8080",
        token: "ns_abc123",
        repo: MyApp.Repo
      )
  """
  def new(base_url, opts) do
    token = Keyword.fetch!(opts, :token)
    repo = Keyword.fetch!(opts, :repo)

    eventodb_client = EventodbEx.Client.new(base_url, token: token)

    %__MODULE__{
      eventodb_client: eventodb_client,
      repo: repo,
      namespace: extract_namespace(token)
    }
  end

  @doc """
  Creates a kit client from an existing EventodbEx.Client.
  """
  def from_eventodb(eventodb_client, opts) do
    repo = Keyword.fetch!(opts, :repo)

    %__MODULE__{
      eventodb_client: eventodb_client,
      repo: repo,
      namespace: extract_namespace(eventodb_client.token)
    }
  end

  @doc """
  Sets a new token for the client.
  """
  def set_token(%__MODULE__{} = kit, token) do
    eventodb_client = EventodbEx.Client.set_token(kit.eventodb_client, token)

    %{kit | eventodb_client: eventodb_client, namespace: extract_namespace(token)}
  end

  # Extract namespace from token (format: ns_<namespace>_<signature>)
  defp extract_namespace(nil), do: nil

  defp extract_namespace(token) when is_binary(token) do
    case String.split(token, "_", parts: 3) do
      ["ns", namespace, _signature] -> namespace
      _ -> nil
    end
  end
end
