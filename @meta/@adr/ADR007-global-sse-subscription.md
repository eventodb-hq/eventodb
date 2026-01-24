# ADR-007: Global SSE Subscription for Namespace-Wide Events

**Date:** 2025-01-24  
**Status:** Proposed  
**Context:** Reducing polling overhead in multi-consumer services

---

## Problem

Services consuming events from EventoDB typically have multiple consumers, each subscribing to a different category:

```
Service: moo_edu
├── Class.Consumer         → polls "class" category
├── ClassMembership.Consumer → polls "class_membership" category  
├── Partnership.Consumer   → polls "partnership" category
└── ContentError.Consumer  → polls "content_error" category
```

**Current behavior:** Each consumer independently polls EventoDB every N seconds.

**Impact:**
- 4 consumers × 1 poll/second = 4 HTTP requests/second per service
- Multiply by number of services = significant unnecessary traffic
- Each poll is a full HTTP request even when no new messages exist

**Alternative considered:** Per-category SSE subscriptions

```
GET /subscribe?category=class&token=...
GET /subscribe?category=class_membership&token=...
GET /subscribe?category=partnership&token=...
GET /subscribe?category=content_error&token=...
```

This reduces polling but requires **N SSE connections** per service (one per category).

---

## Decision

**Add `?all=true` parameter to the SSE `/subscribe` endpoint** to subscribe to all events in a namespace.

```
GET /subscribe?all=true&position=0&token=...
```

This single connection receives poke notifications for **all** writes in the namespace, regardless of category.

---

## Design

### SSE Poke Format (unchanged)

```json
{"stream": "class-123", "position": 5, "globalPosition": 1042}
```

The `stream` field contains the full stream name, from which the client can extract the category:

```
stream: "class-123"        → category: "class"
stream: "class_membership-456" → category: "class_membership"
```

### Client-Side Dispatch

The client maintains a registry of categories it cares about:

```
┌─────────────────────────────────────────────────────────────┐
│                    GlobalSubscriber                         │
│  - Single SSE: /subscribe?all=true&token=...                │
│  - Registered categories: [class, class_membership]         │
└─────────────────────────────────────────────────────────────┘
         │
         │ poke: {stream: "class-123", globalPosition: 1042}
         ▼
    ┌─────────────────────────────────────────────────────────┐
    │ 1. Extract category from stream: "class"                │
    │ 2. Is "class" in registered categories? YES             │
    │ 3. Fetch: category.get("class", position: last_pos)     │
    │ 4. Dispatch messages to Class.Handler                   │
    └─────────────────────────────────────────────────────────┘
```

**Key optimization:** Pokes for unregistered categories are **ignored** (no fetch).

### Traffic Comparison

| Scenario | Before (polling) | Before (per-category SSE) | After (global SSE) |
|----------|------------------|---------------------------|---------------------|
| Connections | 0 | N | **1** |
| Idle traffic | N polls/sec | 0 | **0** |
| On 1 event | N polls | 1 poke | **1 poke** |
| On burst (1 category) | N polls | burst pokes | **burst pokes** |
| On burst (other category) | N polls | 0 | **burst pokes (ignored)** |

The worst case (burst in unrelated category) means receiving pokes that get ignored. A poke is ~100 bytes, so even 10,000 ignored pokes = 1MB - acceptable.

---

## Server Implementation

### SSE Handler Changes (`sse.go`)

```go
func (h *SSEHandler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
    // ... existing setup ...
    
    query := r.URL.Query()
    streamName := query.Get("stream")
    categoryName := query.Get("category")
    subscribeAll := query.Get("all") == "true"  // NEW

    // Validate: need exactly one of stream, category, or all
    if !subscribeAll && streamName == "" && categoryName == "" {
        http.Error(w, "Either 'stream', 'category', or 'all=true' required", http.StatusBadRequest)
        return
    }
    if subscribeAll && (streamName != "" || categoryName != "") {
        http.Error(w, "Cannot combine 'all' with 'stream' or 'category'", http.StatusBadRequest)
        return
    }

    // ... existing validation ...

    if subscribeAll {
        h.subscribeToAll(ctx, w, namespace, position)
        return
    }
    
    // ... existing stream/category handlers ...
}

func (h *SSEHandler) subscribeToAll(ctx context.Context, w http.ResponseWriter, namespace string, startPosition int64) {
    var sub Subscriber
    if h.Pubsub != nil {
        sub = h.Pubsub.SubscribeAll(namespace)
        defer h.Pubsub.UnsubscribeAll(namespace, sub)
    }

    fmt.Fprintf(w, ": ready\n\n")
    if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
    }

    // Note: No initial fetch - client tracks position per category
    
    if h.Pubsub == nil {
        <-ctx.Done()
        return
    }

    for {
        select {
        case <-ctx.Done():
            return
        case event, ok := <-sub:
            if !ok {
                return
            }
            if event.GlobalPosition >= startPosition {
                poke := pokePool.Get().(*Poke)
                poke.Stream = event.Stream
                poke.Position = event.Position
                poke.GlobalPosition = event.GlobalPosition

                err := h.sendPoke(w, poke)
                pokePool.Put(poke)

                if err != nil {
                    return
                }
            }
        }
    }
}
```

### PubSub Changes (`pubsub.go`)

```go
type PubSub struct {
    mu sync.RWMutex
    streamSubs   map[string]map[string]map[Subscriber]struct{}
    categorySubs map[string]map[string]map[Subscriber]struct{}
    allSubs      map[string]map[Subscriber]struct{}  // NEW: namespace -> subscribers
}

func NewPubSub() *PubSub {
    return &PubSub{
        streamSubs:   make(map[string]map[string]map[Subscriber]struct{}),
        categorySubs: make(map[string]map[string]map[Subscriber]struct{}),
        allSubs:      make(map[string]map[Subscriber]struct{}),  // NEW
    }
}

// NEW
func (ps *PubSub) SubscribeAll(namespace string) Subscriber {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    sub := make(Subscriber, 100)

    if ps.allSubs[namespace] == nil {
        ps.allSubs[namespace] = make(map[Subscriber]struct{})
    }
    ps.allSubs[namespace][sub] = struct{}{}

    return sub
}

// NEW
func (ps *PubSub) UnsubscribeAll(namespace string, sub Subscriber) {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    if ps.allSubs[namespace] != nil {
        delete(ps.allSubs[namespace], sub)
        if len(ps.allSubs[namespace]) == 0 {
            delete(ps.allSubs, namespace)
        }
    }
    close(sub)
}

func (ps *PubSub) Publish(event WriteEvent) {
    ps.mu.RLock()
    defer ps.mu.RUnlock()

    // Existing: notify stream subscribers
    // ...

    // Existing: notify category subscribers
    // ...

    // NEW: notify "all" subscribers for this namespace
    if subs := ps.allSubs[event.Namespace]; subs != nil {
        for sub := range subs {
            select {
            case sub <- event:
            default:
                // Channel full, skip
            }
        }
    }
}
```

---

## Client Implementation (MooIntegration)

The client-side implementation is **outside EventoDB** - it lives in the consuming service's integration library.

```elixir
defmodule MooIntegration.GlobalSubscriber do
  use GenServer
  
  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end
  
  def subscribe(category, handler_pid) do
    GenServer.call(__MODULE__, {:subscribe, category, handler_pid})
  end
  
  def init(opts) do
    base_url = Keyword.fetch!(opts, :base_url)
    token = Keyword.fetch!(opts, :token)
    
    # Start SSE connection
    {:ok, _pid} = EventodbEx.subscribe_to_all(base_url, token,
      name: "global-subscriber",
      position: 0,
      on_poke: &handle_poke/1
    )
    
    {:ok, %{handlers: %{}, positions: %{}}}
  end
  
  defp handle_poke(poke) do
    category = extract_category(poke.stream)
    GenServer.cast(__MODULE__, {:poke, category, poke})
  end
  
  def handle_cast({:poke, category, poke}, state) do
    case Map.get(state.handlers, category) do
      nil -> 
        # No handler for this category, ignore
        {:noreply, state}
      handler_pid ->
        # Notify handler to fetch new messages
        send(handler_pid, {:new_events, category, poke.global_position})
        {:noreply, state}
    end
  end
  
  defp extract_category(stream_name) do
    # "class-123" -> "class"
    # "class_membership-456" -> "class_membership"
    case String.split(stream_name, "-", parts: 2) do
      [category, _id] -> category
      [category] -> category
    end
  end
end
```

---

## API Documentation Update

### GET /subscribe

Subscribe to real-time notifications when messages are written.

**Query Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `stream` | string | * | Stream to subscribe to |
| `category` | string | * | Category to subscribe to |
| `all` | boolean | * | Subscribe to all events in namespace |
| `position` | number | No | Starting global position (default: 0) |
| `token` | string | Yes | Authentication token |

*Exactly one of `stream`, `category`, or `all=true` is required.

**Examples:**

```bash
# Subscribe to all events in namespace
curl -N "http://localhost:8080/subscribe?all=true&token=$TOKEN"

# Subscribe to a specific category
curl -N "http://localhost:8080/subscribe?category=account&token=$TOKEN"

# Subscribe to a specific stream
curl -N "http://localhost:8080/subscribe?stream=account-123&token=$TOKEN"
```

---

## Future Enhancement: Server-Side Category Filtering

If the overhead of receiving pokes for unrelated categories becomes problematic, a future enhancement could add server-side filtering:

```
GET /subscribe?categories=class,class_membership&token=...
```

This would require:
1. Parsing comma-separated category list
2. Filtering pokes server-side before sending

This is **not included in this ADR** - the simple `?all=true` approach should be sufficient for most use cases.

---

## Testing

1. **Unit tests:** PubSub `SubscribeAll`/`UnsubscribeAll`/`Publish` behavior
2. **Integration tests:** SSE connection with `?all=true` receives pokes from multiple categories
3. **Black-box tests:** Add to `test_external/` suite

---

## Affected Components

| Component | Change Required |
|-----------|-----------------|
| EventoDB Server (`sse.go`) | Add `?all=true` handling |
| EventoDB Server (`pubsub.go`) | Add `allSubs` map and methods |
| EventoDB API docs | Document new parameter |
| eventodb_ex (Elixir SDK) | Add `subscribe_to_all/3` (optional convenience) |
| eventodb-go (Go SDK) | Add `SubscribeAll` (optional convenience) |

---

## References

- [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md)
- [EventoDB API Reference](../docs/API.md)
- MessageDB legacy design (no `$all` stream, but `global_position` enables similar patterns)
