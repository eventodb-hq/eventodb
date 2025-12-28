defmodule EventodbKit.CodegenConsumerTest do
  use ExUnit.Case, async: false
  import EventodbKit.TestHelper
  alias EventodbKit.TestSupport.EventDispatcher

  defmodule TypeSafeConsumer do
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
      # Use dispatcher for type-safe handling
      case EventDispatcher.dispatch(message["type"], message["data"], &handle_event/2) do
        {:error, :unknown_event} ->
          # Send unknown events to test process
          send(state[:test_pid], {:unknown_event, message["type"]})
          :ok

        {:error, changeset} ->
          # Send validation errors to test process
          send(state[:test_pid], {:validation_error, changeset})
          {:error, :validation_failed}

        {:ok, result} ->
          # Send processed event to test process
          send(state[:test_pid], {:processed, result})
          :ok
      end
    end

    # Pattern match on event module for type-safe handling
    defp handle_event(Events.PartnershipApplicationSubmitted, event) do
      %{
        school: event.school_name,
        email: event.contact_email,
        phone: event.contact_phone
      }
    end

    defp handle_event(Events.PartnershipActivated, event) do
      %{
        school_id: event.school_id,
        claim_code: event.claim_code,
        tier: event.tier
      }
    end

    defp handle_event(Events.ClassJoinRequested, event) do
      %{
        student: event.student_id,
        class: event.class_id
      }
    end

    defp handle_event(_event_module, _event) do
      :handled
    end
  end

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)
    Ecto.Adapters.SQL.Sandbox.mode(EventodbKit.TestRepo, {:shared, self()})

    {kit, namespace_id, token} = create_test_namespace("codegen-consumer")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit, namespace_id: namespace_id, token: token}
  end

  test "consumes and validates PartnershipApplicationSubmitted", %{kit: kit, token: token} do
    # Write valid event directly to EventoDB
    eventodb_client = kit.eventodb_client
    application_id = Ecto.UUID.generate()

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "partnership_application-#{application_id}",
        %{
          type: "PartnershipApplicationSubmitted",
          data: %{
            application_id: application_id,
            school_name: "Springfield Elementary",
            contact_name: "Seymour Skinner",
            contact_email: "principal@springfield.edu",
            contact_phone: "555-0123",
            submitted_at: DateTime.utc_now()
          }
        }
      )

    # Start consumer
    _consumer =
      start_supervised!({
        TypeSafeConsumer,
        [
          namespace: kit.namespace,
          category: "partnership_application",
          consumer_id: "type-safe-consumer",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    # Wait for processed event
    assert_receive {:processed, result}, 500

    assert result.school == "Springfield Elementary"
    assert result.email == "principal@springfield.edu"
    assert result.phone == "555-0123"
  end

  test "handles validation errors gracefully", %{kit: kit, token: token} do
    # Write invalid event to EventoDB
    eventodb_client = kit.eventodb_client

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "partnership_application-invalid",
        %{
          type: "PartnershipApplicationSubmitted",
          data: %{
            application_id: "not-a-uuid",
            school_name: "Test"
            # Missing required fields
          }
        }
      )

    # Start consumer
    _consumer =
      start_supervised!({
        TypeSafeConsumer,
        [
          namespace: kit.namespace,
          category: "partnership_application",
          consumer_id: "validator",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    # Wait for validation error
    assert_receive {:validation_error, changeset}, 500
    assert changeset.errors[:application_id]
    assert changeset.errors[:contact_name]
  end

  test "handles multiple event types", %{kit: kit, token: token} do
    eventodb_client = kit.eventodb_client

    # Write two events in the same category (class_membership)
    student_id = "student-test"
    class_id = "class-test"

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "class_membership-#{class_id}+#{student_id}",
        %{
          type: "ClassJoinRequested",
          data: %{
            student_id: student_id,
            class_id: class_id,
            join_code: "JOIN123",
            requested_at: DateTime.utc_now() |> DateTime.to_iso8601()
          }
        }
      )

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "class_membership-#{class_id}+#{student_id}",
        %{
          type: "ClassMembershipAccepted",
          data: %{
            student_id: student_id,
            class_id: class_id,
            accepted_at: DateTime.utc_now() |> DateTime.to_iso8601(),
            accepted_by: "teacher-123"
          }
        }
      )

    # Start consumer on class_membership category
    _consumer =
      start_supervised!({
        TypeSafeConsumer,
        [
          namespace: kit.namespace,
          category: "class_membership",
          consumer_id: "multi-type",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    # Wait for both events
    assert_receive {:processed, result1}, 500
    assert_receive {:processed, result2}, 500

    # One should be join request, one should be acceptance
    results = [result1, result2]
    assert Enum.any?(results, &match?(%{student: "student-test", class: "class-test"}, &1))
    assert Enum.any?(results, &match?(:handled, &1))
  end

  test "handles ClassJoinRequested event", %{kit: kit, token: token} do
    eventodb_client = kit.eventodb_client
    student_id = "student-test"
    class_id = "class-test"

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "class_membership-#{class_id}+#{student_id}",
        %{
          type: "ClassJoinRequested",
          data: %{
            student_id: student_id,
            class_id: class_id,
            join_code: "JOIN123",
            requested_at: DateTime.utc_now() |> DateTime.to_iso8601()
          }
        }
      )

    # Start consumer on class_membership category
    _consumer =
      start_supervised!({
        TypeSafeConsumer,
        [
          namespace: kit.namespace,
          category: "class_membership",
          consumer_id: "class-handler",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    assert_receive {:processed, result}, 500
    assert result.student == student_id
    assert result.class == class_id
  end

  test "handles unknown event types", %{kit: kit, token: token} do
    eventodb_client = kit.eventodb_client

    {:ok, _result, _client} =
      EventodbEx.stream_write(
        eventodb_client,
        "test-stream",
        %{
          type: "UnknownEvent",
          data: %{foo: "bar"}
        }
      )

    # Start consumer on generic category
    _consumer =
      start_supervised!({
        TypeSafeConsumer,
        [
          namespace: kit.namespace,
          category: "test",
          consumer_id: "unknown-handler",
          base_url: base_url(),
          token: token,
          repo: EventodbKit.TestRepo,
          poll_interval: 100,
          batch_size: 10,
          test_pid: self()
        ]
      })

    assert_receive {:unknown_event, "UnknownEvent"}, 500
  end
end
