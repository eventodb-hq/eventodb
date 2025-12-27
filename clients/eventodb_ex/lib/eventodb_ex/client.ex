defmodule EventodbEx.Client do
  @moduledoc """
  Low-level HTTP/RPC client for EventoDB.
  """

  alias EventodbEx.Error

  @type t :: %__MODULE__{
          base_url: String.t(),
          token: String.t() | nil,
          req: Req.Request.t()
        }

  defstruct [:base_url, :token, :req]

  @doc """
  Creates a new EventoDB client.

  ## Options

    * `:token` - Authentication token (optional)

  ## Examples

      iex> client = EventodbEx.Client.new("http://localhost:8080", token: "ns_...")

  """
  @spec new(String.t(), keyword()) :: t()
  def new(base_url, opts \\ []) do
    token = Keyword.get(opts, :token)

    req =
      Req.new(
        base_url: base_url,
        headers: build_headers(token),
        retry: false
      )

    %__MODULE__{
      base_url: base_url,
      token: token,
      req: req
    }
  end

  @doc """
  Makes an RPC call to the EventoDB server.

  Returns `{:ok, result, updated_client}` on success, where the client
  may be updated with a captured token from the response.

  Returns `{:error, error}` on failure.
  """
  @spec rpc(t(), String.t(), list()) :: {:ok, any(), t()} | {:error, Error.t()}
  def rpc(client, method, args \\ []) do
    body = [method | args]

    case Req.post(client.req, url: "/rpc", json: body) do
      {:ok, %{status: 200, body: result, headers: headers}} ->
        new_client = maybe_capture_token(client, headers)
        {:ok, result, new_client}

      {:ok, %{body: %{"error" => _} = error_body}} ->
        {:error, Error.from_response(error_body)}

      {:ok, %{status: status, body: body}} ->
        {:error, Error.network_error("HTTP #{status}: #{inspect(body)}")}

      {:error, %Mint.TransportError{reason: :econnrefused}} ->
        {:error, Error.network_error("Connection refused")}

      {:error, exception} ->
        {:error, Error.network_error(Exception.message(exception))}
    end
  end

  @doc """
  Gets the current authentication token.
  """
  @spec get_token(t()) :: String.t() | nil
  def get_token(%__MODULE__{token: token}), do: token

  @doc """
  Sets the authentication token and rebuilds the request.
  """
  @spec set_token(t(), String.t()) :: t()
  def set_token(client, token) do
    req =
      Req.new(
        base_url: client.base_url,
        headers: build_headers(token),
        retry: false
      )

    %{client | token: token, req: req}
  end

  # Private helpers

  defp build_headers(nil) do
    [{"content-type", "application/json"}]
  end

  defp build_headers(token) do
    [
      {"content-type", "application/json"},
      {"authorization", "Bearer #{token}"}
    ]
  end

  defp maybe_capture_token(client, headers) when is_map(headers) do
    # Req returns headers as a map with lowercase keys
    case Map.get(headers, "x-eventodb-token") do
      [token] when is_nil(client.token) ->
        set_token(client, token)

      _ ->
        client
    end
  end

  defp maybe_capture_token(client, _headers), do: client
end
