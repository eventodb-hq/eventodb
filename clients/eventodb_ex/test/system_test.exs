defmodule EventodbEx.SystemTest do
  use ExUnit.Case, async: true
  import EventodbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("system")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "SYS-001: Get server version", %{client: client} do
    assert {:ok, version, _client} = EventodbEx.system_version(client)
    assert is_binary(version)
    # Should match semver pattern (but allow "dev" for development builds)
    if !String.match?(version, ~r/^\d+\.\d+\.\d+/) do
      IO.puts("Warning: version '#{version}' doesn't match semver pattern (may be dev version)")
    end
  end

  test "SYS-002: Get server health", %{client: client} do
    assert {:ok, health, _client} = EventodbEx.system_health(client)
    assert is_map(health)
    assert health.status == "ok"
  end
end
