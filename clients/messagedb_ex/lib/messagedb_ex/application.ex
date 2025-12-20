defmodule MessagedbEx.Application do
  @moduledoc false

  use Application

  @impl true
  def start(_type, _args) do
    children = [
      {Registry, keys: :unique, name: MessagedbEx.Registry}
    ]

    opts = [strategy: :one_for_one, name: MessagedbEx.Supervisor]
    Supervisor.start_link(children, opts)
  end
end
