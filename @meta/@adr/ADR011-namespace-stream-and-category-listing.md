# ADR-011: Namespace Stream and Category Listing API

**Date:** 2026-02-28  
**Status:** Proposed  
**Context:** Required by EventodbWeb introspection UI (see MemoMoo ADR080)

---

## Problem

There is currently no way to enumerate what streams or categories exist in a namespace. The only read operations are:

- `stream.get` — requires knowing the stream name upfront
- `category.get` — requires knowing the category name upfront
- `ns.info` — returns `messageCount` and `streamCount` (counts only, no names)

This makes any kind of browser or introspection UI impossible: you can look things up by name, but you cannot discover what names exist.

---

## Decision

Add two new RPC methods: `ns.streams` and `ns.categories`.

Both are read-only, scoped to the authenticated namespace, and follow the existing API conventions (compact JSON arrays for lists, cursor-based pagination).

---

## New Methods

### `ns.streams`

List streams in the current namespace with optional prefix filtering and cursor-based pagination.

**Request:**
```json
["ns.streams", {
  "prefix": "account",
  "limit": 100,
  "cursor": "account-122"
}]
```

**Options:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `prefix` | string | No | `""` | Filter streams whose name starts with this string |
| `limit` | number | No | 100 | Max streams to return (max 1000) |
| `cursor` | string | No | `""` | Pagination cursor — return streams after this stream name (exclusive) |

**Response:**
```json
[
  {"stream": "account-123", "version": 5, "lastActivity": "2026-01-15T10:30:00Z"},
  {"stream": "account-456", "version": 2, "lastActivity": "2026-01-16T09:12:00Z"}
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `stream` | string | Full stream name |
| `version` | number | Current stream version (position of last message, 0-based) |
| `lastActivity` | string | ISO 8601 UTC timestamp of last write |

Results are sorted lexicographically by stream name. An empty array means no streams match (not an error).

**Error Codes:**
- `INVALID_REQUEST` — invalid options
- `AUTH_REQUIRED` — no token

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '["ns.streams", {"prefix": "account", "limit": 50}]'
```

---

### `ns.categories`

List distinct categories in the current namespace, with stream and message counts.

**Request:**
```json
["ns.categories"]
```

No options. Returns all categories in the namespace.

**Response:**
```json
[
  {"category": "account", "streamCount": 42, "messageCount": 1500},
  {"category": "order",   "streamCount": 8,  "messageCount": 230},
  {"category": "user",    "streamCount": 200, "messageCount": 4100}
]
```

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `category` | string | Category name (portion of stream name before first `-`) |
| `streamCount` | number | Number of distinct streams in this category |
| `messageCount` | number | Total messages across all streams in this category |

Results are sorted lexicographically by category name.

Category extraction follows existing `category.get` semantics: the category is the portion of the stream name before the first `-`. A stream with no `-` (e.g. a bare `account`) is its own category.

**Error Codes:**
- `AUTH_REQUIRED` — no token

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '["ns.categories"]'
```

---

## Implementation

### Go server (`golang/`)

**1. `internal/store/store.go` — extend the `Store` interface**

```go
// ListStreams returns streams in a namespace with optional filtering and pagination.
ListStreams(ctx context.Context, namespace string, opts *ListStreamsOpts) ([]*StreamInfo, error)

// ListCategories returns distinct categories in a namespace with counts.
ListCategories(ctx context.Context, namespace string) ([]*CategoryInfo, error)
```

New types in `store.go`:

```go
type ListStreamsOpts struct {
    Prefix string // filter by stream name prefix
    Limit  int64  // max results (default 100, max 1000)
    Cursor string // pagination: return streams after this name (exclusive)
}

type StreamInfo struct {
    StreamName   string
    Version      int64
    LastActivity time.Time
}

type CategoryInfo struct {
    Category     string
    StreamCount  int64
    MessageCount int64
}
```

**2. `internal/store/sqlite/read.go` — SQLite implementation**

```sql
-- ListStreams
SELECT stream_name,
       MAX(position)                              AS version,
       MAX(time)                                  AS last_activity
FROM messages
WHERE (:prefix = '' OR stream_name LIKE :prefix || '%')
  AND (:cursor = '' OR stream_name > :cursor)
GROUP BY stream_name
ORDER BY stream_name ASC
LIMIT :limit;

-- ListCategories
SELECT substr(stream_name, 1,
         CASE WHEN instr(stream_name, '-') > 0
              THEN instr(stream_name, '-') - 1
              ELSE length(stream_name) END)       AS category,
       COUNT(DISTINCT stream_name)                AS stream_count,
       COUNT(*)                                   AS message_count
FROM messages
GROUP BY category
ORDER BY category ASC;
```

**3. `internal/store/postgres/read.go` — Postgres implementation**

```sql
-- ListStreams
SELECT stream_name,
       MAX(position)                              AS version,
       MAX(time)                                  AS last_activity
FROM "{SCHEMA}".messages
WHERE ($1 = '' OR stream_name LIKE $1 || '%')
  AND ($2 = '' OR stream_name > $2)
GROUP BY stream_name
ORDER BY stream_name ASC
LIMIT $3;

-- ListCategories
SELECT split_part(stream_name, '-', 1)            AS category,
       COUNT(DISTINCT stream_name)                AS stream_count,
       COUNT(*)                                   AS message_count
FROM "{SCHEMA}".messages
GROUP BY category
ORDER BY category ASC;
```

**4. `internal/api/handlers.go` — two new handlers**

Follow the exact same pattern as `handleNamespaceInfo`: extract namespace from context, call store, format response.

**5. `internal/api/rpc.go` — register the methods**

```go
h.registerMethod("ns.streams",    h.handleNamespaceStreams)
h.registerMethod("ns.categories", h.handleNamespaceCategories)
```

**6. `docs/API.md` — document the two new methods** (same style as existing entries)

---

## SDK updates

Both SDKs must be updated to expose the new methods. Neither requires any structural change — it is purely additive.

### `eventodb_ex` (Elixir, `backend_v2/deps/eventodb_ex/`)

Add to `lib/eventodb_ex.ex`:

```elixir
@doc """
Lists streams in the current namespace.

## Options
  * `:prefix` - filter by stream name prefix
  * `:limit` - max results (default 100)
  * `:cursor` - pagination cursor (stream name, exclusive)
"""
@spec namespace_streams(Client.t(), map()) ::
        {:ok, list(map()), Client.t()} | {:error, Error.t()}
def namespace_streams(client, opts \\ %{}) do
  with {:ok, result, client} <- Client.rpc(client, "ns.streams", [opts]) do
    {:ok, Enum.map(result, &snake_case_keys/1), client}
  end
end

@doc """
Lists distinct categories in the current namespace with stream and message counts.
"""
@spec namespace_categories(Client.t()) ::
        {:ok, list(map()), Client.t()} | {:error, Error.t()}
def namespace_categories(client) do
  with {:ok, result, client} <- Client.rpc(client, "ns.categories", []) do
    {:ok, Enum.map(result, &snake_case_keys/1), client}
  end
end
```

The `normalize_options` function already handles camelCase conversion for any map keys, so no changes needed there.

### `eventodb_kit` (Elixir, `clients/eventodb_kit/`)

Add to `lib/eventodb_kit.ex` (delegating to `EventodbEx`, same pattern as `namespace_info`):

```elixir
@doc """
Lists streams in the current namespace.
"""
def namespace_streams(%Client{} = kit, opts \\ %{}) do
  case EventodbEx.namespace_streams(kit.eventodb_client, opts) do
    {:ok, streams, client} -> {:ok, streams, %{kit | eventodb_client: client}}
    {:error, reason} -> {:error, reason}
  end
end

@doc """
Lists distinct categories in the current namespace.
"""
def namespace_categories(%Client{} = kit) do
  case EventodbEx.namespace_categories(kit.eventodb_client) do
    {:ok, categories, client} -> {:ok, categories, %{kit | eventodb_client: client}}
    {:error, reason} -> {:error, reason}
  end
end
```

### TypeScript test client (`test_external/lib/client.ts`)

Add types and methods:

```typescript
export interface StreamInfo {
  stream: string;
  version: number;
  lastActivity: string;
}

export interface CategoryInfo {
  category: string;
  streamCount: number;
  messageCount: number;
}

export interface ListStreamsOptions {
  prefix?: string;
  limit?: number;
  cursor?: string;
}

// In EventoDBClient class:

async listStreams(opts?: ListStreamsOptions): Promise<StreamInfo[]> {
  return this.rpc('ns.streams', opts || {});
}

async listCategories(): Promise<CategoryInfo[]> {
  return this.rpc('ns.categories');
}
```

---

## Testing

### Go integration tests (`golang/test_integration/`)

New file `namespace_listing_test.go`:

- `ns.streams` returns empty array for empty namespace
- `ns.streams` returns all streams after writes
- `ns.streams` prefix filter works correctly
- `ns.streams` cursor pagination is consistent (no duplicates, no gaps)
- `ns.streams` version and lastActivity are accurate
- `ns.categories` returns empty array for empty namespace
- `ns.categories` correctly derives categories from stream names
- `ns.categories` counts are accurate after multiple writes
- `ns.categories` streams with no `-` appear as their own category
- Both methods are namespace-scoped (writes in ns-A don't appear in ns-B)

### External tests (`test_external/tests/`)

New file `listing.test.ts` covering the same cases via the TypeScript client.

---

## Pagination design rationale

`ns.streams` uses **keyset/cursor pagination** (stream name as cursor) rather than offset pagination, because:

1. Stream names are stable sort keys — no drift when new streams are added during pagination
2. Consistent with how `stream.get` and `category.get` use position-based cursors
3. Simple to implement in both SQLite and Postgres without `OFFSET` performance issues at scale

`ns.categories` has **no pagination** because the number of distinct categories is expected to be small (tens, not thousands) in any realistic namespace. If this assumption proves wrong we can add it later.

---

## References

- [ADR-001: RPC-Style API Format](./ADR001-api-format.md) — API conventions followed here
- [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md) — namespace scoping
- [ADR-010: Namespace Schema Versioning](./ADR010-namespace-schema-versioning.md) — migration pattern for any SQL changes
- MemoMoo ADR080 — EventodbWeb UI that motivated this ADR
