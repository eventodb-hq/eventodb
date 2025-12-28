defmodule EventodbKit.ConsumerTest do
  use ExUnit.Case, async: false
  import EventodbKit.TestHelper
  alias EventodbKit.Consumer.Position
  alias EventodbKit.Consumer.Idempotency

  defmodule TestConsumer do
    use EventodbKit.Consumer

    def start_link(opts) do
      EventodbKit.Consumer.start_link(__MODULE__, opts)
    end

    def child_spec(opts) do
      %{
        id: __MODULE__,
        start: {__MODULE__, :start_link, [opts]},
        type: :worker,
        restart: :permanent
      }
    end

    @impl EventodbKit.Consumer
    def init(opts) do
      {:ok, opts}
    end

    @impl EventodbKit.Consumer
    def handle_message(message, state) do
      # Send to test process
      send(state[:test_pid], {:message_received, message})
      :ok
    end
  end

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)

    {kit, namespace_id, token} = create_test_namespace("consumer")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit, namespace_id: namespace_id, token: token}
  end

  test "processes messages from category", %{kit: kit, token: token} do
    # Write some events directly to EventoDB
    eventodb_client = kit.eventodb_client

    {:ok, _result, _client} =
      EventodbEx.stream_write(eventodb_client, "partnership-1", %{
        type: "LeadCreated",
        data: %{id: 1}
      })

    {:ok, _result, _client} =
      EventodbEx.stream_write(eventodb_client, "partnership-2", %{
        type: "LeadCreated",
        data: %{id: 2}
      })

    # Use shared mode for consumer test
    Ecto.Adapters.SQL.Sandbox.mode(EventodbKit.TestRepo, {:shared, self()})

    # Start consumer
    _consumer =
      start_supervised!({
        TestConsumer,
        [
          namespace: kit.namespace,
          category: "partnership",
          consumer_id: "test-consumer",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    # Wait for messages
    assert_receive {:message_received, msg1}, 500
    assert_receive {:message_received, msg2}, 500

    assert msg1["type"] == "LeadCreated"
    assert msg2["type"] == "LeadCreated"
  end

  test "tracks position", %{kit: kit} do
    # Save position
    Position.save(EventodbKit.TestRepo, kit.namespace, "partnership", "consumer-1", 100)

    # Load position
    position = Position.load(EventodbKit.TestRepo, kit.namespace, "partnership", "consumer-1")
    assert position == 100

    # Load non-existent position (should return 0)
    position2 = Position.load(EventodbKit.TestRepo, kit.namespace, "partnership", "consumer-2")
    assert position2 == 0
  end

  test "prevents duplicate processing", %{kit: kit} do
    event_id = Ecto.UUID.generate()

    # Check not processed
    refute Idempotency.processed?(EventodbKit.TestRepo, event_id)

    # Mark as processed
    Idempotency.mark_processed(
      EventodbKit.TestRepo,
      event_id,
      kit.namespace,
      "LeadCreated",
      "partnership",
      "consumer-1"
    )

    # Check processed
    assert Idempotency.processed?(EventodbKit.TestRepo, event_id)
  end
end
