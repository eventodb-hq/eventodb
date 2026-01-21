defmodule EventodbEx.MixProject do
  use Mix.Project

  def project do
    [
      app: :eventodb_ex,
      version: "0.1.0",
      elixir: "~> 1.18",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      description: "Elixir client for EventoDB - a simple, fast message store",
      package: package(),
      source_url: "https://github.com/yourusername/eventodb-ex"
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {EventodbEx.Application, []}
    ]
  end

  defp deps do
    [
      {:req, "~> 0.4 or ~> 0.5 or ~> 0.6 or ~> 1.0"},
      {:jason, "~> 1.4"},
      {:mint, "~> 1.5"}
    ]
  end

  defp package do
    [
      licenses: ["MIT"],
      links: %{"GitHub" => "https://github.com/yourusername/eventodb-ex"}
    ]
  end
end
