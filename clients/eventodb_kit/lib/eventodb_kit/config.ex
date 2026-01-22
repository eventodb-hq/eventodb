defmodule EventodbKit.Config do
  @moduledoc """
  Configuration helpers for EventodbKit.
  """

  @doc """
  Returns whether SQL logging is enabled.

  Defaults to `false` to reduce noise. Set to `true` for debugging:

      config :eventodb_kit, :log_sql, true
  """
  def log_sql? do
    Application.get_env(:eventodb_kit, :log_sql, false)
  end
end
