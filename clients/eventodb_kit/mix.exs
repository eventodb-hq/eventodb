defmodule EventodbKit.MixProject do
  use Mix.Project

  def project do
    [
      app: :eventodb_kit,
      version: "0.1.0",
      elixir: "~> 1.18",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      aliases: aliases(),
      elixirc_paths: elixirc_paths(Mix.env()),
      description: "Production-ready Elixir SDK for EventoDB with resilience patterns",
      package: package()
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
      # Path dependency initially (until eventodb_ex is released to Hex)
      {:eventodb_ex, path: "../eventodb_ex"},

      # Core dependencies
      {:ecto_sql, "~> 3.12"},
      {:postgrex, "~> 0.19"},
      {:jason, "~> 1.4"},

      # Dev/test
      {:ex_doc, "~> 0.34", only: :dev, runtime: false}
    ]
  end

  defp elixirc_paths(:test), do: ["lib", "test/support"]
  defp elixirc_paths(_), do: ["lib"]

  defp aliases do
    [
      test: ["test"]
    ]
  end

  defp package do
    [
      licenses: ["MIT"],
      links: %{"GitHub" => "https://github.com/eventodb/eventodb-kit"}
    ]
  end
end
