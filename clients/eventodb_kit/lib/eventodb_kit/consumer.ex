defmodule EventodbKit.Consumer do
  @moduledoc """
  Behaviour and base implementation for EventoDB consumers.

  Consumers automatically track position and handle idempotency.
  They run as singletons using Chosen.

  ## Example

      defmodule MyApp.PartnershipConsumer do
        use EventodbKit.Consumer

        def start_link(opts) do
          EventodbKit.Consumer.start_link(__MODULE__, opts)
        end

        @impl EventodbKit.Consumer
        def init(opts) do
          {:ok, %{
            namespace: "analytics",
            category: "partnership",
            consumer_id: "singleton",
            base_url: "http://localhost:8080",
            token: System.get_env("ANALYTICS_TOKEN"),
            repo: MyApp.Repo,
            poll_interval: 1_000,
            batch_size: 100
          }}
        end

        @impl EventodbKit.Consumer
        def handle_message(message, state) do
          case message.type do
            "PartnershipApplicationSubmitted" ->
              # Process message
              :ok

            _ ->
              Logger.warn("Unknown event type: \#{message.type}")
              :ok
          end
        end
      end
  """

  use GenServer
  require Logger

  alias EventodbKit.Consumer.Position
  alias EventodbKit.Consumer.Idempotency

  @callback init(opts :: keyword()) :: {:ok, state :: map()}
  @callback handle_message(message :: map(), state :: map()) :: :ok | {:error, term()}

  defmacro __using__(_opts) do
    quote do
      @behaviour EventodbKit.Consumer
    end
  end

  @default_poll_interval 1_000
  @default_batch_size 100

  def start_link(module, opts) do
    GenServer.start_link(__MODULE__, {module, opts})
  end

  @impl true
  def init({module, opts}) do
    {:ok, consumer_state} = module.init(opts)

    # Convert to map if it's a keyword list
    consumer_state =
      if Keyword.keyword?(consumer_state) do
        Enum.into(consumer_state, %{})
      else
        consumer_state
      end

    namespace = Map.fetch!(consumer_state, :namespace)
    category = Map.fetch!(consumer_state, :category)
    consumer_id = Map.fetch!(consumer_state, :consumer_id)
    base_url = Map.fetch!(consumer_state, :base_url)
    token = Map.fetch!(consumer_state, :token)
    repo = Map.fetch!(consumer_state, :repo)
    poll_interval = Map.get(consumer_state, :poll_interval, @default_poll_interval)
    batch_size = Map.get(consumer_state, :batch_size, @default_batch_size)

    # Load current position
    position = Position.load(repo, namespace, category, consumer_id)

    eventodb_client = EventodbEx.Client.new(base_url, token: token)

    state = %{
      module: module,
      consumer_state: consumer_state,
      namespace: namespace,
      category: category,
      consumer_id: consumer_id,
      repo: repo,
      poll_interval: poll_interval,
      batch_size: batch_size,
      position: position,
      eventodb_client: eventodb_client,
      group_member: Map.get(consumer_state, :group_member),
      group_size: Map.get(consumer_state, :group_size)
    }

    # Schedule first poll
    schedule_poll(state)

    {:ok, state}
  end

  @impl true
  def handle_info(:poll, state) do
    state = poll_and_process(state)
    schedule_poll(state)
    {:noreply, state}
  end

  # Private functions

  defp schedule_poll(state) do
    Process.send_after(self(), :poll, state.poll_interval)
  end

  defp poll_and_process(state) do
    opts = %{
      position: state.position,
      batch_size: state.batch_size
    }

    case EventodbEx.category_get(state.eventodb_client, state.category, opts) do
      {:ok, messages, _client} ->
        process_messages(state, messages)

      {:error, reason} ->
        Logger.error("Consumer: Failed to fetch messages: #{inspect(reason)}")
        state
    end
  end

  defp process_messages(state, messages) do
    # Filter by group membership if applicable
    messages =
      if state.group_member != nil and state.group_size != nil do
        filter_by_group(messages, state.group_member, state.group_size)
      else
        messages
      end

    Enum.reduce(messages, state, fn message, acc_state ->
      process_message(acc_state, message)
    end)
  end

  defp filter_by_group(messages, group_member, group_size) do
    Enum.filter(messages, fn message ->
      # Message format: [id, stream, type, data, global_position, metadata, time, stream_position]
      [_id, stream, _type, _data, _gpos, _metadata, _time, _spos] = message
      stream_hash = :erlang.phash2(stream)
      rem(stream_hash, group_size) == group_member
    end)
  end

  defp process_message(state, message) do
    # Message format from EventodbEx: [id, stream, type, data, global_position, metadata, time, stream_position]
    [event_id, stream, type, data, global_position, metadata, _time, _stream_position] = message

    # Convert to map for handler
    message_map = %{
      "id" => event_id,
      "stream_name" => stream,
      "type" => type,
      "data" => data,
      "global_position" => global_position,
      "metadata" => metadata
    }

    # Check if already processed
    if Idempotency.processed?(state.repo, event_id) do
      Logger.debug("Consumer: Skipping already processed event #{event_id}")
      update_position(state, global_position)
    else
      # Process message
      case state.module.handle_message(message_map, state.consumer_state) do
        :ok ->
          # Mark as processed and update position
          Idempotency.mark_processed(
            state.repo,
            event_id,
            state.namespace,
            type,
            state.category,
            state.consumer_id
          )

          update_position(state, global_position)

        {:error, reason} ->
          Logger.error("Consumer: Failed to process message #{event_id}: #{inspect(reason)}")
          state
      end
    end
  end

  defp update_position(state, global_position) do
    Position.save(
      state.repo,
      state.namespace,
      state.category,
      state.consumer_id,
      global_position
    )

    %{state | position: global_position}
  end
end
