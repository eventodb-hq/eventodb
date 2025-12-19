defmodule MessagedbEx.MixProject do
  use Mix.Project

  def project do
    [
      app: :messagedb_ex,
      version: "0.1.0",
      elixir: "~> 1.18",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      description: "Elixir client for MessageDB - a simple, fast message store",
      package: package(),
      source_url: "https://github.com/yourusername/messagedb-ex"
    ]
  end

  def application do
    [
      extra_applications: [:logger]
    ]
  end

  defp deps do
    [
      {:req, "~> 0.4.0"},
      {:jason, "~> 1.4"}
    ]
  end

  defp package do
    [
      licenses: ["MIT"],
      links: %{"GitHub" => "https://github.com/yourusername/messagedb-ex"}
    ]
  end
end
