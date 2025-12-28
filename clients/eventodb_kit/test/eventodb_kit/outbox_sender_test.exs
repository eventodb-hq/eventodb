defmodule EventodbKit.OutboxSenderTest do
  use ExUnit.Case, async: true
  import EventodbKit.TestHelper
  alias EventodbKit.Schema.Outbox
  alias EventodbKit.Outbox, as: OutboxOps

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)

    {kit, namespace_id, token} = create_test_namespace("outbox-sender")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit, namespace_id: namespace_id, token: token}
  end

  test "sends unsent events to EventoDB", %{kit: kit, token: token} do
    # Write to outbox
    stream = "account-123"
    message = %{type: "Deposited", data: %{amount: 100}}
    {:ok, outbox_id, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Start sender and allow sandbox access
    {:ok, sender} =
      start_supervised!({
        EventodbKit.OutboxSender,
        [
          namespace: kit.namespace,
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10
        ]
      })
      |> tap(fn pid -> Ecto.Adapters.SQL.Sandbox.allow(EventodbKit.TestRepo, self(), pid) end)
      |> then(fn pid -> {:ok, pid} end)

    # Wait for sender to process
    Process.sleep(300)

    # Verify message was sent
    outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
    assert outbox.sent_at != nil

    # Verify message is in EventoDB
    {:ok, messages, _kit} = EventodbKit.stream_get(kit, stream)
    assert length(messages) == 1
    # EventodbEx returns messages as arrays: [id, type, position, global_position, data, metadata, time]
    [_id, type, _position, _global_position, data, _metadata, _time] = hd(messages)
    assert type == "Deposited"
    assert data["amount"] == 100
  end

  test "marks events as sent", %{kit: kit} do
    # Write to outbox
    {:ok, outbox_id, _kit} =
      EventodbKit.stream_write(kit, "stream-1", %{type: "Event1", data: %{}})

    # Mark as sent
    OutboxOps.mark_sent(EventodbKit.TestRepo, outbox_id)

    # Verify
    outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
    assert outbox.sent_at != nil
  end

  test "fetches unsent messages", %{kit: kit} do
    # Write multiple messages
    {:ok, _id1, _kit} =
      EventodbKit.stream_write(kit, "stream-1", %{type: "Event1", data: %{}})

    {:ok, id2, _kit} = EventodbKit.stream_write(kit, "stream-2", %{type: "Event2", data: %{}})

    # Mark one as sent
    OutboxOps.mark_sent(EventodbKit.TestRepo, id2)

    # Fetch unsent
    unsent = OutboxOps.fetch_unsent(EventodbKit.TestRepo, kit.namespace)
    assert length(unsent) == 1
    assert hd(unsent).stream == "stream-1"
  end
end
