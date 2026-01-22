# ADR-006: Plaintext Namespace Extraction in SDKs

**Date:** 2025-01-22  
**Status:** Accepted  
**Context:** SDKs require namespace for local operations but currently expose base64-encoded namespace from token

---

## Problem

The token format embeds the namespace as base64url-encoded:

```
ns_<base64url(namespace)>_<random_hex>

Example: ns_ZGVmYXVsdA_0000000000000000...
         │    │
         │    └── base64url("default") = "ZGVmYXVsdA"
         └── prefix
```

Current SDK implementations extract the namespace but **do not decode it**:

```elixir
# EventodbKit.Client (current - WRONG)
defp extract_namespace(token) do
  case String.split(token, "_", parts: 3) do
    ["ns", namespace, _signature] -> namespace  # Returns "ZGVmYXVsdA" (encoded!)
    _ -> nil
  end
end
```

This causes confusion because:

1. **Local database stores encoded namespace** - The `evento_outbox.namespace` and `evento_consumer_positions.namespace` columns contain base64 strings like `"ZGVmYXVsdA"` instead of `"default"`

2. **Debugging is harder** - Developers see encoded values in their database and logs

3. **Separate namespace parameter feels needed** - Users pass both `token` and `namespace` because they don't trust the encoded value

4. **Inconsistency** - The EventoDB server decodes the namespace internally, but SDKs don't

---

## Decision

**All SDKs must decode the base64url namespace from the token to plaintext.**

The token is self-describing and contains all information needed. SDKs should:

1. Extract the namespace from the token
2. Decode it from base64url to plaintext
3. Use the plaintext namespace for all local operations
4. **Remove the separate `namespace` parameter** from SDK APIs

---

## Implementation

### Principle: Decode Once, Reuse Everywhere

The namespace must be decoded **exactly once** when the client is created, then stored and reused for all subsequent operations. No repeated decoding on every request.

```elixir
# Client struct caches the decoded namespace
defstruct [:eventodb_client, :repo, :namespace]

def new(base_url, opts) do
  token = Keyword.fetch!(opts, :token)
  %__MODULE__{
    # ...
    namespace: extract_namespace(token)  # Decoded ONCE here, reused everywhere
  }
end
```

### Elixir SDK (EventodbKit)

```elixir
# BEFORE (wrong)
defp extract_namespace(token) when is_binary(token) do
  case String.split(token, "_", parts: 3) do
    ["ns", namespace, _signature] -> namespace  # Returns encoded
    _ -> nil
  end
end

# AFTER (correct)
defp extract_namespace(token) when is_binary(token) do
  case String.split(token, "_", parts: 3) do
    ["ns", encoded_namespace, _signature] ->
      case Base.url_decode64(encoded_namespace, padding: false) do
        {:ok, decoded} -> decoded
        :error -> nil
      end
    _ -> nil
  end
end
```

### Go SDK (eventodb-go)

```go
// BEFORE (wrong)
func ExtractNamespace(token string) (string, error) {
    parts := strings.Split(token, "_")
    if len(parts) != 3 || parts[0] != "ns" {
        return "", errors.New("invalid token format")
    }
    return parts[1], nil  // Returns encoded
}

// AFTER (correct)
func ExtractNamespace(token string) (string, error) {
    parts := strings.Split(token, "_")
    if len(parts) != 3 || parts[0] != "ns" {
        return "", errors.New("invalid token format")
    }
    decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
    if err != nil {
        return "", fmt.Errorf("invalid namespace encoding: %w", err)
    }
    return string(decoded), nil
}
```

### TypeScript SDK

```typescript
// BEFORE (wrong)
function extractNamespace(token: string): string | null {
  const parts = token.split('_');
  if (parts.length !== 3 || parts[0] !== 'ns') return null;
  return parts[1];  // Returns encoded
}

// AFTER (correct)
function extractNamespace(token: string): string | null {
  const parts = token.split('_');
  if (parts.length !== 3 || parts[0] !== 'ns') return null;
  try {
    // base64url decode (no padding)
    const encoded = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    return atob(encoded);
  } catch {
    return null;
  }
}
```

---

## API Changes

### Remove Separate Namespace Parameter

**Before:**
```elixir
# Confusing - namespace passed separately AND embedded in token
EventodbKit.OutboxSender.start_link(
  namespace: "ZGVmYXVsdA",  # Why do I need this?
  token: "ns_ZGVmYXVsdA_xxx...",
  base_url: "http://localhost:8080",
  repo: MyRepo
)
```

**After:**
```elixir
# Clean - token is the source of truth
EventodbKit.OutboxSender.start_link(
  token: "ns_ZGVmYXVsdA_xxx...",  # Namespace extracted automatically
  base_url: "http://localhost:8080",
  repo: MyRepo
)
```

---

## Migration

For existing installations with encoded namespaces in local database:

1. **Option A: Data migration** - Update existing records to decode namespaces
2. **Option B: Fresh start** - Clear outbox/position tables (acceptable for most cases)

Migration script example (Elixir):

```elixir
# priv/repo/migrations/xxx_decode_namespaces.exs
defmodule MyApp.Repo.Migrations.DecodeNamespaces do
  use Ecto.Migration

  def up do
    # Decode base64 namespaces in outbox table
    execute """
    UPDATE evento_outbox 
    SET namespace = decode(namespace, 'base64')::text
    WHERE namespace ~ '^[A-Za-z0-9_-]+$' 
      AND namespace NOT LIKE '%-%'
    """
    
    # Decode base64 namespaces in consumer positions
    execute """
    UPDATE evento_consumer_positions
    SET namespace = decode(namespace, 'base64')::text
    WHERE namespace ~ '^[A-Za-z0-9_-]+$'
      AND namespace NOT LIKE '%-%'
    """
  end

  def down do
    # Re-encode if needed (not recommended)
  end
end
```

---

## Affected Components

| Component | Change Required |
|-----------|-----------------|
| `eventodb_kit` (Elixir) | Decode namespace in `Client.extract_namespace/1` |
| `eventodb-go` (Go SDK) | Decode namespace in client |
| `eventodb-ts` (TypeScript) | Decode namespace in client |
| `EventodbKit.OutboxSender` | Remove `namespace` option |
| `EventodbKit.Consumer` | Remove `namespace` option |
| `Integration.Supervisor` | Remove `namespace` from common_opts |
| Test helpers | Use plaintext namespace |

---

## Benefits

1. **Single source of truth** - Token contains everything needed
2. **Readable database** - Local tables show `"default"` not `"ZGVmYXVsdA"`
3. **Simpler API** - No redundant `namespace` parameter
4. **Easier debugging** - Logs and queries use human-readable names
5. **Consistency** - SDKs behave same as server (which always decodes)

---

## References

- [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md) - Token format specification
- [RFC 4648: Base64url encoding](https://tools.ietf.org/html/rfc4648#section-5)
