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

    # Load non-existent position (should return nil for fresh consumer)
    position2 = Position.load(EventodbKit.TestRepo, kit.namespace, "partnership", "consumer-2")
    assert position2 == nil
  end

  test "position semantics: fresh consumer starts at position 0", %{kit: kit} do
    # Fresh consumer has no position saved
    position = Position.load(EventodbKit.TestRepo, kit.namespace, "test-category", "fresh-consumer")
    assert position == nil

    # When position is nil, consumer should fetch from position 0 (start of stream)
    # This is handled in poll_and_process: fetch_position = if state.position, do: state.position + 1, else: 0
    fetch_position = if position, do: position + 1, else: 0
    assert fetch_position == 0
  end

  test "position semantics: after processing event, fetch starts after last position", %{kit: kit} do
    # Simulate: consumer processed event at global_position 5
    Position.save(EventodbKit.TestRepo, kit.namespace, "test-category", "test-consumer", 5)

    # Load saved position - this is the LAST PROCESSED position
    position = Position.load(EventodbKit.TestRepo, kit.namespace, "test-category", "test-consumer")
    assert position == 5

    # When fetching, we need position + 1 to get events AFTER the last processed one
    # EventoDB's position parameter is inclusive, so position=5 would re-fetch the same event
    fetch_position = if position, do: position + 1, else: 0
    assert fetch_position == 6
  end

  test "position semantics: processes each event exactly once", %{kit: kit, token: token} do
    eventodb_client = kit.eventodb_client

    # Write 3 events to streams in "postest" category
    # Category is derived from stream name prefix before the hyphen
    {:ok, _, _} = EventodbEx.stream_write(eventodb_client, "postest-1", %{type: "Event1", data: %{}})
    {:ok, _, _} = EventodbEx.stream_write(eventodb_client, "postest-2", %{type: "Event2", data: %{}})
    {:ok, _, _} = EventodbEx.stream_write(eventodb_client, "postest-3", %{type: "Event3", data: %{}})

    Ecto.Adapters.SQL.Sandbox.mode(EventodbKit.TestRepo, {:shared, self()})

    # Start consumer
    _consumer =
      start_supervised!({
        TestConsumer,
        [
          namespace: kit.namespace,
          category: "postest",
          consumer_id: "position-test-consumer",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 50,
          batch_size: 10,
          test_pid: self()
        ]
      })

    # Should receive exactly 3 messages
    assert_receive {:message_received, %{"type" => "Event1"}}, 500
    assert_receive {:message_received, %{"type" => "Event2"}}, 500
    assert_receive {:message_received, %{"type" => "Event3"}}, 500

    # Wait a bit for additional poll cycles - should NOT receive duplicates
    refute_receive {:message_received, _}, 200
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
