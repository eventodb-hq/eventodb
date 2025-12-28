defmodule EventodbKit.OutboxSender do
  @moduledoc """
  Background worker that sends unsent outbox messages to EventoDB.
  Runs as a singleton using Chosen.
  """

  use GenServer
  require Logger
  alias EventodbKit.Outbox

  @default_poll_interval 1_000
  @default_batch_size 100

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts)
  end

  @impl true
  def init(opts) do
    namespace = Keyword.fetch!(opts, :namespace)
    base_url = Keyword.fetch!(opts, :base_url)
    token = Keyword.fetch!(opts, :token)
    repo = Keyword.fetch!(opts, :repo)
    poll_interval = Keyword.get(opts, :poll_interval, @default_poll_interval)
    batch_size = Keyword.get(opts, :batch_size, @default_batch_size)

    eventodb_client = EventodbEx.Client.new(base_url, token: token)

    state = %{
      namespace: namespace,
      eventodb_client: eventodb_client,
      repo: repo,
      poll_interval: poll_interval,
      batch_size: batch_size
    }

    # Schedule first poll
    schedule_poll(state)

    {:ok, state}
  end

  @impl true
  def handle_info(:poll, state) do
    send_unsent_messages(state)
    schedule_poll(state)
    {:noreply, state}
  end

  # Private functions

  defp schedule_poll(state) do
    Process.send_after(self(), :poll, state.poll_interval)
  end

  defp send_unsent_messages(state) do
    messages = Outbox.fetch_unsent(state.repo, state.namespace, batch_size: state.batch_size)

    if length(messages) > 0 do
      Logger.debug("OutboxSender: Found #{length(messages)} unsent messages")

      Enum.each(messages, fn outbox ->
        case send_message(state, outbox) do
          :ok ->
            Outbox.mark_sent(state.repo, outbox.id)

          {:error, reason} ->
            Logger.error("OutboxSender: Failed to send message #{outbox.id}: #{inspect(reason)}")
        end
      end)
    end
  end

  defp send_message(state, outbox) do
    message = build_message(outbox)
    opts = build_opts(outbox)

    case EventodbEx.stream_write(state.eventodb_client, outbox.stream, message, opts) do
      {:ok, _result, _client} ->
        :ok

      {:error, reason} ->
        {:error, reason}
    end
  end

  defp build_message(outbox) do
    msg = %{
      type: outbox.type,
      data: outbox.data
    }

    if outbox.metadata do
      Map.put(msg, :metadata, outbox.metadata)
    else
      msg
    end
  end

  defp build_opts(outbox) do
    opts = %{}

    opts =
      if outbox.write_options do
        case Map.get(outbox.write_options, "expected_version") do
          nil -> opts
          version -> Map.put(opts, :expected_version, version)
        end
      else
        opts
      end

    opts =
      if outbox.write_options do
        case Map.get(outbox.write_options, "id") do
          nil -> opts
          id -> Map.put(opts, :id, id)
        end
      else
        opts
      end

    opts
  end
end
