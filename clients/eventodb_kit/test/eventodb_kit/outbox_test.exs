defmodule EventodbKit.OutboxTest do
  use ExUnit.Case, async: true
  import EventodbKit.TestHelper
  alias EventodbKit.Schema.Outbox

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)
    {kit, namespace_id, _token} = create_test_namespace("outbox")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit}
  end

  test "writes message to outbox", %{kit: kit} do
    stream = "account-123"
    message = %{type: "Deposited", data: %{amount: 100}}

    {:ok, outbox_id, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Verify in database
    outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
    assert outbox.namespace == kit.namespace
    assert outbox.stream == stream
    assert outbox.type == "Deposited"
    assert outbox.data == %{"amount" => 100}
    assert is_nil(outbox.sent_at)
  end

  test "prevents duplicate events with idempotency key", %{kit: kit} do
    stream = "payment-456"

    message = %{
      type: "PaymentRequested",
      data: %{
        amount: 1000,
        idempotency_key: "payment_user123_invoice456"
      }
    }

    # Write once
    {:ok, id1, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Write again with same idempotency key
    {:ok, id2, _kit} = EventodbKit.stream_write(kit, stream, message)

    # Should return existing record
    assert id1 == id2

    # Only one record in database
    count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
    assert count == 1
  end

  test "transactional write with business logic", %{kit: kit} do
    EventodbKit.TestRepo.transaction(fn ->
      # Simulate business logic
      lead_id = Ecto.UUID.generate()

      # Write to outbox in same transaction
      {:ok, _outbox_id, _kit} =
        EventodbKit.stream_write(
          kit,
          "partnership-#{lead_id}",
          %{type: "LeadCreated", data: %{lead_id: lead_id}}
        )

      # Both succeed or both rollback
      :ok
    end)

    count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
    assert count == 1
  end

  test "batch write", %{kit: kit} do
    messages = [
      {"stream-1", %{type: "Event1", data: %{}}},
      {"stream-2", %{type: "Event2", data: %{}}}
    ]

    {:ok, outbox_ids, _kit} = EventodbKit.stream_write_batch(kit, messages)

    assert length(outbox_ids) == 2

    count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
    assert count == 2
  end

  test "write with expected_version option", %{kit: kit} do
    stream = "account-456"
    message = %{type: "Deposited", data: %{amount: 200}}

    {:ok, outbox_id, _kit} =
      EventodbKit.stream_write(kit, stream, message, expected_version: 0)

    outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
    assert outbox.write_options["expected_version"] == 0
  end
end
