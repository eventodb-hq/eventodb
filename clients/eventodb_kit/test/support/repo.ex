defmodule EventodbKit.TestRepo do
  use Ecto.Repo,
    otp_app: :eventodb_kit,
    adapter: Ecto.Adapters.Postgres
end
