defmodule EventodbKit.CodegenIntegrationTest do
  use ExUnit.Case, async: true
  import EventodbKit.TestHelper
  alias EventodbKit.TestSupport.{EventPublisher, EventDispatcher}
  alias EventodbKit.Schema.Outbox

  setup do
    :ok = Ecto.Adapters.SQL.Sandbox.checkout(EventodbKit.TestRepo)
    {kit, namespace_id, _token} = create_test_namespace("codegen")

    on_exit(fn ->
      cleanup_namespace(namespace_id)
    end)

    %{kit: kit}
  end

  describe "EventPublisher with code-generated events" do
    test "publishes valid PartnershipApplicationSubmitted event", %{kit: kit} do
      application_id = Ecto.UUID.generate()

      event_data = %{
        application_id: application_id,
        school_name: "Springfield Elementary",
        contact_name: "Seymour Skinner",
        contact_email: "principal@springfield.edu",
        contact_phone: "555-0123",
        submitted_at: DateTime.utc_now()
      }

      {:ok, outbox_id, _kit} =
        EventPublisher.publish(
          kit,
          Events.PartnershipApplicationSubmitted,
          event_data
        )

      # Verify in outbox
      outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
      assert outbox.type == "PartnershipApplicationSubmitted"
      assert outbox.stream == "partnership_application-#{application_id}"
      assert outbox.data["school_name"] == "Springfield Elementary"
      assert outbox.data["contact_email"] == "principal@springfield.edu"
    end

    test "validates email format", %{kit: kit} do
      event_data = %{
        application_id: Ecto.UUID.generate(),
        school_name: "Test School",
        contact_name: "John Doe",
        contact_email: "invalid-email",
        contact_phone: "555-0123",
        submitted_at: DateTime.utc_now()
      }

      {:error, changeset} =
        EventPublisher.publish(
          kit,
          Events.PartnershipApplicationSubmitted,
          event_data
        )

      assert changeset.errors[:contact_email]
    end

    test "validates required fields", %{kit: kit} do
      event_data = %{
        application_id: Ecto.UUID.generate(),
        school_name: "Test School"
        # Missing required fields
      }

      {:error, changeset} =
        EventPublisher.publish(
          kit,
          Events.PartnershipApplicationSubmitted,
          event_data
        )

      assert changeset.errors[:contact_name]
      assert changeset.errors[:contact_email]
    end

    test "publishes with metadata", %{kit: kit} do
      event_data = %{
        application_id: Ecto.UUID.generate(),
        school_name: "Test School",
        contact_name: "John Doe",
        contact_email: "john@test.com",
        contact_phone: "555-0123",
        submitted_at: DateTime.utc_now()
      }

      {:ok, outbox_id, _kit} =
        EventPublisher.publish(
          kit,
          Events.PartnershipApplicationSubmitted,
          event_data,
          metadata: %{user_id: "admin-123", source: "web"}
        )

      outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
      assert outbox.metadata == %{"user_id" => "admin-123", "source" => "web"}
    end

    test "publishes with expected_version", %{kit: kit} do
      event_data = %{
        application_id: Ecto.UUID.generate(),
        school_name: "Test School",
        contact_name: "John Doe",
        contact_email: "john@test.com",
        contact_phone: "555-0123",
        submitted_at: DateTime.utc_now()
      }

      {:ok, outbox_id, _kit} =
        EventPublisher.publish(
          kit,
          Events.PartnershipApplicationSubmitted,
          event_data,
          expected_version: 0
        )

      outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
      assert outbox.write_options["expected_version"] == 0
    end

    test "publishes PartnershipActivated event", %{kit: kit} do
      school_id = "school-#{unique_suffix()}"
      application_id = Ecto.UUID.generate()

      event_data = %{
        school_id: school_id,
        application_id: application_id,
        claim_code: "CLAIM-123",
        activated_at: DateTime.utc_now() |> DateTime.to_iso8601(),
        tier: "premium",
        activated_by: "admin@example.com"
      }

      {:ok, outbox_id, _kit} =
        EventPublisher.publish(
          kit,
          Events.PartnershipActivated,
          event_data
        )

      outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
      assert outbox.type == "PartnershipActivated"
      assert outbox.stream == "partnership-#{school_id}"
      assert outbox.data["claim_code"] == "CLAIM-123"
      assert outbox.data["tier"] == "premium"
    end

    test "publishes ClassJoinRequested event", %{kit: kit} do
      student_id = "student-#{unique_suffix()}"
      class_id = "class-#{unique_suffix()}"

      event_data = %{
        student_id: student_id,
        class_id: class_id,
        join_code: "JOIN123",
        requested_at: DateTime.utc_now() |> DateTime.to_iso8601()
      }

      {:ok, outbox_id, _kit} =
        EventPublisher.publish(
          kit,
          Events.ClassJoinRequested,
          event_data
        )

      outbox = EventodbKit.TestRepo.get!(Outbox, outbox_id)
      assert outbox.type == "ClassJoinRequested"
      assert String.contains?(outbox.stream, class_id)
      assert String.contains?(outbox.stream, student_id)
    end
  end

  describe "EventDispatcher for type-safe consuming" do
    test "dispatches known event with validation" do
      event_data = %{
        "application_id" => Ecto.UUID.generate(),
        "school_name" => "Test School",
        "contact_name" => "John Doe",
        "contact_email" => "john@test.com",
        "contact_phone" => "555-0123",
        "submitted_at" => DateTime.utc_now()
      }

      result =
        EventDispatcher.dispatch(
          "PartnershipApplicationSubmitted",
          event_data,
          fn event_module, validated ->
            assert event_module == Events.PartnershipApplicationSubmitted
            assert validated.school_name == "Test School"
            assert validated.contact_email == "john@test.com"
            :processed
          end
        )

      assert result == {:ok, :processed}
    end

    test "returns error for unknown event" do
      result =
        EventDispatcher.dispatch(
          "UnknownEvent",
          %{},
          fn _, _ -> :should_not_be_called end
        )

      assert result == {:error, :unknown_event}
    end

    test "returns validation error for invalid data" do
      event_data = %{
        "application_id" => "invalid-uuid",
        "school_name" => "Test"
        # Missing required fields
      }

      result =
        EventDispatcher.dispatch(
          "PartnershipApplicationSubmitted",
          event_data,
          fn _, _ -> :should_not_be_called end
        )

      assert {:error, changeset} = result
      assert changeset.errors[:application_id]
    end

    test "dispatches different event types" do
      # PartnershipActivated
      event_data1 = %{
        "school_id" => "school-test",
        "application_id" => Ecto.UUID.generate(),
        "claim_code" => "CLAIM-123",
        "activated_at" => DateTime.utc_now() |> DateTime.to_iso8601(),
        "tier" => "premium",
        "activated_by" => "admin@example.com"
      }

      result1 =
        EventDispatcher.dispatch(
          "PartnershipActivated",
          event_data1,
          fn event_module, validated ->
            assert event_module == Events.PartnershipActivated
            assert validated.claim_code == "CLAIM-123"
            assert validated.tier == "premium"
            :ok
          end
        )

      assert result1 == {:ok, :ok}

      # ClassJoinRequested
      event_data2 = %{
        "student_id" => "student-test",
        "class_id" => "class-test",
        "join_code" => "JOIN123",
        "requested_at" => DateTime.utc_now() |> DateTime.to_iso8601()
      }

      result2 =
        EventDispatcher.dispatch(
          "ClassJoinRequested",
          event_data2,
          fn event_module, validated ->
            assert event_module == Events.ClassJoinRequested
            assert validated.join_code == "JOIN123"
            :ok
          end
        )

      assert result2 == {:ok, :ok}
    end
  end

  describe "Transactional workflow" do
    test "publishes event in transaction with business logic", %{kit: kit} do
      application_id = Ecto.UUID.generate()

      result =
        EventodbKit.TestRepo.transaction(fn ->
          # Simulate business logic (would be actual database insert)
          business_data = %{
            id: application_id,
            school_name: "Test School",
            status: :pending
          }

          # Publish event in same transaction
          event_data = %{
            application_id: application_id,
            school_name: business_data.school_name,
            contact_name: "John Doe",
            contact_email: "john@test.com",
            contact_phone: "555-0123",
            submitted_at: DateTime.utc_now()
          }

          case EventPublisher.publish(kit, Events.PartnershipApplicationSubmitted, event_data) do
            {:ok, _outbox_id, _kit} ->
              business_data

            {:error, changeset} ->
              EventodbKit.TestRepo.rollback({:validation_failed, changeset})
          end
        end)

      assert {:ok, business_data} = result
      assert business_data.id == application_id

      # Verify event in outbox
      count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
      assert count == 1
    end

    test "rolls back transaction on validation failure", %{kit: kit} do
      result =
        EventodbKit.TestRepo.transaction(fn ->
          # Invalid event data
          event_data = %{
            application_id: "invalid-uuid",
            school_name: "Test"
          }

          case EventPublisher.publish(kit, Events.PartnershipApplicationSubmitted, event_data) do
            {:ok, _outbox_id, _kit} ->
              :success

            {:error, changeset} ->
              EventodbKit.TestRepo.rollback({:validation_failed, changeset})
          end
        end)

      assert {:error, {:validation_failed, changeset}} = result
      assert changeset.errors[:application_id]

      # No events in outbox
      count = EventodbKit.TestRepo.aggregate(Outbox, :count, :id)
      assert count == 0
    end
  end
end
