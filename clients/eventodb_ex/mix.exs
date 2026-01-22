defmodule EventodbEx.MixProject do
  use Mix.Project

  @version "0.2.0"
  @source_url "https://github.com/eventodb-hq/eventodb"

  def project do
    [
      app: :eventodb_ex,
      version: @version,
      elixir: "~> 1.14",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      description: description(),
      package: package(),
      name: "EventodbEx",
      source_url: @source_url,
      docs: docs()
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
      {:mint, "~> 1.5"},
      {:ex_doc, "~> 0.31", only: :dev, runtime: false}
    ]
  end

  defp description do
    """
    Elixir client for EventoDB - a high-performance event store/message store.
    Supports stream operations, category queries, consumer groups, SSE subscriptions,
    namespace management, and optimistic locking.
    """
  end

  defp package do
    [
      name: "eventodb_ex",
      licenses: ["MIT"],
      links: %{
        "GitHub" => @source_url,
        "Documentation" => "https://hexdocs.pm/eventodb_ex"
      },
      files: ~w(lib .formatter.exs mix.exs README.md LICENSE CHANGELOG.md)
    ]
  end

  defp docs do
    [
      main: "readme",
      extras: ["README.md", "CHANGELOG.md", "LICENSE"],
      source_ref: "v#{@version}",
      source_url_pattern: "#{@source_url}/blob/main/clients/eventodb_ex/%{path}#L%{line}"
    ]
  end
end
