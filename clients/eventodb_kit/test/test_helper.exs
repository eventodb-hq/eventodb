# Configure test database
Application.put_env(:eventodb_kit, EventodbKit.TestRepo,
  username: System.get_env("POSTGRES_USER", "postgres"),
  password: System.get_env("POSTGRES_PASSWORD", "postgres"),
  hostname: System.get_env("POSTGRES_HOST", "localhost"),
  port: String.to_integer(System.get_env("POSTGRES_PORT", "5432")),
  database: "eventodb_kit_test",
  pool: Ecto.Adapters.SQL.Sandbox,
  pool_size: 10
)

# Create database (ignore if already exists)
_ = Ecto.Adapters.Postgres.storage_down(EventodbKit.TestRepo.config())

case Ecto.Adapters.Postgres.storage_up(EventodbKit.TestRepo.config()) do
  :ok -> :ok
  {:error, :already_up} -> :ok
  error -> raise "Failed to create database: #{inspect(error)}"
end

# Start test repo
{:ok, _} = EventodbKit.TestRepo.start_link()

# Run migrations using EventodbKit.Migration (before setting sandbox mode)
Ecto.Migrator.run(
  EventodbKit.TestRepo,
  [{1, EventodbKit.Migration}],
  :up,
  all: true,
  log: false
)

# Set sandbox mode after migrations
Ecto.Adapters.SQL.Sandbox.mode(EventodbKit.TestRepo, :manual)

ExUnit.start()
