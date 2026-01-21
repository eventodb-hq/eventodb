defmodule EventodbKit.MixProject do
  use Mix.Project

  @version "0.1.0"
  @source_url "https://github.com/eventodb-hq/eventodb"

  def project do
    [
      app: :eventodb_kit,
      version: @version,
      elixir: "~> 1.14",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      aliases: aliases(),
      elixirc_paths: elixirc_paths(Mix.env()),
      description: description(),
      package: package(),
      name: "EventodbKit",
      source_url: @source_url,
      docs: docs()
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {EventodbKit.Application, []}
    ]
  end

  defp deps do
    [
      eventodb_ex_dep(),

      # Core dependencies
      {:ecto_sql, "~> 3.12"},
      {:postgrex, "~> 0.19"},
      {:jason, "~> 1.4"},

      # Dev/test
      {:ex_doc, "~> 0.31", only: :dev, runtime: false}
    ]
  end

  # Local development: uses path dependency (sibling folder)
  # Hex release: uses published eventodb_ex
  #
  # How it works:
  # - Default: path dependency for local dev (when ../eventodb_ex exists)
  # - EVENTODB_HEX=1: force Hex dependency (for testing before release)
  # - When published to Hex: path won't exist, falls back to Hex
  defp eventodb_ex_dep do
    use_hex = System.get_env("EVENTODB_HEX") == "1"
    local_exists = File.exists?(Path.expand("../eventodb_ex/mix.exs", __DIR__))

    if local_exists and not use_hex do
      {:eventodb_ex, path: "../eventodb_ex"}
    else
      {:eventodb_ex, "~> 0.1"}
    end
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  defp aliases do
    [
      test: ["test"]
    ]
  end

  defp description do
    """
    Production-ready Elixir SDK for EventoDB with resilience patterns.
    Provides outbox pattern, consumer position tracking, idempotency,
    and background workers built on top of EventodbEx.
    """
  end

  defp package do
    [
      name: "eventodb_kit",
      licenses: ["MIT"],
      links: %{
        "GitHub" => @source_url,
        "Documentation" => "https://hexdocs.pm/eventodb_kit"
      },
      files: ~w(lib .formatter.exs mix.exs README.md LICENSE CHANGELOG.md)
    ]
  end

  defp docs do
    [
      main: "readme",
      extras: ["README.md", "CHANGELOG.md", "LICENSE"],
      source_ref: "v#{@version}",
      source_url_pattern: "#{@source_url}/blob/main/clients/eventodb_kit/%{path}#L%{line}"
    ]
  end
end
