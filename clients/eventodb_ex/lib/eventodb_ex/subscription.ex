defmodule EventodbEx.Subscription do
  @moduledoc """
  SSE subscription for real-time message notifications.
  """

  use GenServer
  require Logger

  ## Public API

  def start_link(opts) do
    name = Keyword.fetch!(opts, :name)
    via_name = {:via, Registry, {EventodbEx.Registry, name}}
    GenServer.start_link(__MODULE__, opts, name: via_name)
  end

  def close(name) when is_binary(name) do
    case Registry.lookup(EventodbEx.Registry, name) do
      [{pid, _}] -> GenServer.stop(pid, :normal)
      [] -> {:error, :not_found}
    end
  end

  ## GenServer callbacks

  @impl true
  def init(opts) do
    url = Keyword.fetch!(opts, :url)
    on_poke = Keyword.fetch!(opts, :on_poke)
    on_error = Keyword.get(opts, :on_error)

    uri = URI.parse(url)
    scheme = String.to_existing_atom(uri.scheme || "http")
    port = uri.port || (if scheme == :https, do: 443, else: 80)

    case Mint.HTTP.connect(scheme, uri.host, port) do
      {:ok, conn} ->
        path = build_path(uri)
        headers = [
          {"accept", "text/event-stream"},
          {"cache-control", "no-cache"}
        ]

        case Mint.HTTP.request(conn, "GET", path, headers, nil) do
          {:ok, conn, request_ref} ->
            state = %{
              conn: conn,
              request_ref: request_ref,
              on_poke: on_poke,
              on_error: on_error,
              buffer: ""
            }

            {:ok, state}

          {:error, conn, reason} ->
            Mint.HTTP.close(conn)
            {:stop, {:connection_error, reason}}
        end

      {:error, reason} ->
        {:stop, {:connection_error, reason}}
    end
  end

  @impl true
  def handle_info(message, state) do
    case Mint.HTTP.stream(state.conn, message) do
      {:ok, conn, responses} ->
        state = %{state | conn: conn}
        state = handle_responses(responses, state)
        {:noreply, state}

      {:error, _conn, error, _responses} ->
        if state.on_error, do: state.on_error.(error)
        {:stop, :normal, state}

      :unknown ->
        {:noreply, state}
    end
  end

  @impl true
  def terminate(_reason, state) when is_map(state) do
    if Map.has_key?(state, :conn) do
      Mint.HTTP.close(state.conn)
    end
    :ok
  end

  def terminate(_reason, _state), do: :ok

  ## Private

  defp build_path(uri) do
    path = uri.path || "/"
    if uri.query, do: "#{path}?#{uri.query}", else: path
  end

  defp handle_responses(responses, state) do
    Enum.reduce(responses, state, fn response, acc ->
      handle_response(response, acc)
    end)
  end

  defp handle_response({:status, _ref, _status}, state), do: state
  defp handle_response({:headers, _ref, _headers}, state), do: state

  defp handle_response({:data, _ref, data}, state) do
    buffer = state.buffer <> data
    process_buffer(buffer, state)
  end

  defp handle_response({:done, _ref}, state) do
    {:stop, :normal, state}
  end

  defp handle_response(_response, state), do: state

  defp process_buffer(buffer, state) do
    case parse_sse_event(buffer) do
      {:event, event, rest} ->
        if event.type == "poke" do
          handle_poke_event(event.data, state)
        end

        process_buffer(rest, %{state | buffer: rest})

      {:incomplete, rest} ->
        %{state | buffer: rest}
    end
  end

  defp parse_sse_event(buffer) do
    case String.split(buffer, "\n\n", parts: 2) do
      [event_block, rest] when event_block != "" ->
        event = parse_event_block(event_block)
        {:event, event, rest}

      _ ->
        {:incomplete, buffer}
    end
  end

  defp parse_event_block(block) do
    lines = String.split(block, "\n")

    Enum.reduce(lines, %{type: nil, data: nil}, fn line, acc ->
      case String.split(line, ": ", parts: 2) do
        ["event", type] -> %{acc | type: type}
        ["data", data] -> %{acc | data: data}
        _ -> acc
      end
    end)
  end

  defp handle_poke_event(data, state) when is_binary(data) do
    case Jason.decode(data) do
      {:ok, poke_data} ->
        poke = %{
          stream: poke_data["stream"],
          position: poke_data["position"],
          global_position: poke_data["globalPosition"]
        }

        state.on_poke.(poke)

      {:error, _reason} ->
        if state.on_error do
          state.on_error.({:parse_error, data})
        end
    end

    state
  end
end
