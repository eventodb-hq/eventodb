defmodule MessagedbEx.SystemTest do
  use ExUnit.Case, async: false
  import MessagedbEx.TestHelper

  setup do
    {client, namespace_id, _token} = create_test_namespace("system")

    on_exit(fn ->
      cleanup_namespace(client, namespace_id)
    end)

    %{client: client}
  end

  test "SYS-001: Get server version", %{client: client} do
    assert {:ok, version, _client} = MessagedbEx.system_version(client)
    assert is_binary(version)
    # Should match semver pattern
    assert String.match?(version, ~r/^\d+\.\d+\.\d+/)
  end

  test "SYS-002: Get server health", %{client: client} do
    assert {:ok, health, _client} = MessagedbEx.system_health(client)
    assert is_map(health)
    assert health.status == "ok"
  end
end
