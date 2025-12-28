defmodule EventodbKit.Migration do
  @moduledoc """
  Migrations for EventodbKit tables.

  To use, create a migration in your application:

      defmodule MyApp.Repo.Migrations.AddEventodbKitTables do
        use Ecto.Migration

        def up do
          EventodbKit.Migration.up(version: 1)
        end

        def down do
          EventodbKit.Migration.down(version: 1)
        end
      end

  Then run: `mix ecto.migrate`
  """

  use Ecto.Migration

  @initial_version 1
  @current_version 1

  def up(opts \\ []) do
    version = Keyword.get(opts, :version, @current_version)
    initial = Keyword.get(opts, :initial, @initial_version)

    for v <- initial..version do
      apply_up(v)
    end
  end

  def down(opts \\ []) do
    version = Keyword.get(opts, :version, @initial_version)
    current = Keyword.get(opts, :current, @current_version)

    for v <- current..version//-1 do
      apply_down(v)
    end
  end

  defp apply_up(1) do
    create_if_not_exists table(:evento_outbox, primary_key: false) do
      add(:id, :uuid, primary_key: true, default: fragment("gen_random_uuid()"))
      add(:namespace, :string, null: false)
      add(:stream, :string, null: false)
      add(:type, :string, null: false)
      add(:data, :map, null: false)
      add(:metadata, :map)
      add(:write_options, :map)

      add(:sent_at, :utc_datetime_usec)
      add(:created_at, :utc_datetime_usec, null: false, default: fragment("NOW()"))
    end

    create_if_not_exists index(:evento_outbox, [:namespace, :created_at],
      where: "sent_at IS NULL",
      name: :evento_outbox_unsent_idx
    )

    execute("""
    CREATE INDEX IF NOT EXISTS evento_outbox_idempotency_key_idx
    ON evento_outbox ((data->>'idempotency_key'))
    """)

    create_if_not_exists index(:evento_outbox, [:sent_at])

    create_if_not_exists table(:evento_consumer_positions, primary_key: false) do
      add(:namespace, :string, null: false, primary_key: true)
      add(:category, :string, null: false, primary_key: true)
      add(:consumer_id, :string, null: false, primary_key: true)
      add(:position, :bigint, null: false, default: 0)
      add(:updated_at, :utc_datetime)
    end

    create_if_not_exists index(:evento_consumer_positions, [:updated_at])

    create_if_not_exists table(:evento_processed_events, primary_key: false) do
      add(:event_id, :uuid, primary_key: true)
      add(:namespace, :string, null: false)
      add(:event_type, :string, null: false)
      add(:category, :string, null: false)
      add(:consumer_id, :string, null: false)
      add(:processed_at, :utc_datetime, null: false, default: fragment("NOW()"))
    end

    create_if_not_exists index(:evento_processed_events, [:namespace, :processed_at])
  end

  defp apply_down(1) do
    drop_if_exists index(:evento_processed_events, [:namespace, :processed_at])
    drop_if_exists table(:evento_processed_events)

    drop_if_exists index(:evento_consumer_positions, [:updated_at])
    drop_if_exists table(:evento_consumer_positions)

    drop_if_exists index(:evento_outbox, [:sent_at])

    execute("DROP INDEX IF EXISTS evento_outbox_idempotency_key_idx")

    drop_if_exists index(:evento_outbox, [:namespace, :created_at],
      where: "sent_at IS NULL",
      name: :evento_outbox_unsent_idx
    )

    drop_if_exists table(:evento_outbox)
  end
end
