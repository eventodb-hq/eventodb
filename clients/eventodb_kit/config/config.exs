import Config

# Only configure in test environment
if config_env() == :test do
  config :eventodb_kit, EventodbKit.TestRepo,
    username: System.get_env("POSTGRES_USER", "postgres"),
    password: System.get_env("POSTGRES_PASSWORD", "postgres"),
    hostname: System.get_env("POSTGRES_HOST", "localhost"),
    port: String.to_integer(System.get_env("POSTGRES_PORT", "5432")),
    database: "eventodb_kit_test",
    pool: Ecto.Adapters.SQL.Sandbox,
    pool_size: 10,
    log: false

  config :eventodb_kit, ecto_repos: [EventodbKit.TestRepo]


end
