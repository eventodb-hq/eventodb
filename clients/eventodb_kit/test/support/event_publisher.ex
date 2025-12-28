defmodule EventodbKit.TestSupport.EventPublisher do
  @moduledoc """
  Helper for publishing validated code-generated events to EventodbKit.
  """

  @doc """
  Publishes a validated event to the outbox.

  ## Example

      EventPublisher.publish(
        kit,
        Events.PartnershipApplicationSubmitted,
        event_data
      )
  """
  def publish(kit, event_module, data, opts \\ []) do
    with {:ok, validated} <- event_module.validate!(data) do
      stream = event_module.stream_name(validated)
      event_type = event_module |> Module.split() |> List.last()

      message = %{
        type: event_type,
        data: Map.from_struct(validated)
      }

      message =
        case Keyword.get(opts, :metadata) do
          nil -> message
          metadata -> Map.put(message, :metadata, metadata)
        end

      write_opts = Keyword.take(opts, [:expected_version, :id])

      EventodbKit.stream_write(kit, stream, message, write_opts)
    end
  end
end
