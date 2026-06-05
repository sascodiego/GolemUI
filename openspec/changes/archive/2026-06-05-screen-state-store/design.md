# Design: Screen State Store

## Technical Approach

Introduce a per-screen `ScreenState` (thread-safe `map[string]any`) threaded through the `Compose` recursion via a private `composeWithState`. Inputs write to the store on change. Buttons with `submit_action` publish a `Snapshot()` to a fixed `eventbus.SubmitChannel`. Data grids subscribe to `SubmitChannel` and dispatch by `FilterMode`: server-mode passes snapshot values as positional `$1, $2, …` args to `BusinessPool.Query`; client-mode filters pre-loaded `masterRows` in-memory. The public `Compose(node)` signature is preserved; it internally creates the state and delegates.

## Architecture Decisions

### AD-1: Per-screen state threaded through Compose

| Option | Tradeoff | Decision |
|--------|----------|----------|
| A1: `*ScreenState` param in recursive `composeWithState` | Explicit ownership, testable, no globals | **Chosen** |
| A2: Global map `ui.ScreenStates[viewID]` | No signature change | Rejected: hidden global state, leaks across screens |
| A3: `context.Context` value | Idiomatic Go | Rejected: `context.WithValue` misuse for non-request data |

### AD-2: Snapshot payload as `map[string]any`

| Option | Tradeoff | Decision |
|--------|----------|----------|
| B1: Raw `map[string]any` snapshot | Extensible, no struct to maintain | **Chosen** |
| B2: Typed `SubmitPayload` struct | Discoverable, IDE-friendly | Rejected: premature; wrapper can be added later |

### AD-3: Fixed `eventbus.SubmitChannel` constant

| Option | Tradeoff | Decision |
|--------|----------|----------|
| C1: Fixed constant `"screen:submit"` | Simpler, unified SUBMIT per screen | **Chosen** |
| C2: Per-button `submit_action` as channel | Reuses existing field | Rejected: diverges from unified SUBMIT model |

### AD-4: `FilterMode` field on NodeMeta

| Option | Tradeoff | Decision |
|--------|----------|----------|
| D1: `FilterMode` on grid's `NodeMeta` | Declarative, backwards-compatible | **Chosen** |
| D2: Per-input `filter_mode` | Fine-grained | Rejected: conceptually wrong — the grid owns the mode decision |

### AD-5: Eager master-buffer load in Compose

| Option | Tradeoff | Decision |
|--------|----------|----------|
| E1: Eager at Compose time | Predictable latency | **Chosen** |
| E2: Lazy on first SUBMIT | Faster initial render | Rejected: first SUBMIT is slow, defeats "load once" promise |

### AD-6: Breaking change — remove old `bind_to` direct wiring

The existing `text_input.OnChanged → Publish(bind_to, value)` and `data_grid.Subscribe(bind_to, ...)` paths are **removed**. Grid reactivity now requires a button with `submit_action`. Old screens that relied on keystroke-level filtering must be updated to include a submit button. This is a clean break — no hybrid compatibility mode.

**Rationale**: Supporting both paths doubles the code paths and test surface for no long-term benefit. The proposal explicitly chose to remove the old path.

## Data Flow

```
                         SUBMIT Flow (Server Mode)
                         ═════════════════════════

  ┌──────────┐  OnChanged   ┌──────────────┐
  │text_input├──────────────►│              │
  │ bind_to= │  state.Set   │ *ScreenState │
  │ "title"  │  ("title",v) │              │
  └──────────┘              │  map[string] │   ┌──────────┐
  ┌──────────┐  OnChanged   │     any      │   │ EventBus │
  │text_input├──────────────►│              │   │          │
  │ bind_to= │  state.Set   │ + RWMutex    │   │          │
  │ "amount" │  ("amount",v)│              │   │          │
  └──────────┘              └──────┬───────┘   │          │
                                  │            │          │
  ┌──────────┐  OnTapped          │ Snapshot() │          │
  │  button  ├────────────────────┘            │          │
  │submit_   │  Publish(SubmitChannel,         │          │
  │action=   │    snapshot)                    │          │
  │"submit"  ├────────────────────────────────►│          │
  └──────────┘                                 └─────┬────┘
                                                     │
                                    Subscribe(SubmitChannel)
                                                     │
  ┌──────────┐     ┌─────────────────────┐           │
  │data_grid │     │  Server Mode:       │           │
  │filter_   │◄────│  args = snapshot    │◄──────────┘
  │mode=     │     │  values by key      │
  │"server"  │     │  order              │
  └────┬─────┘     └─────────────────────┘
       │
       ▼
  BusinessPool.Query(ctx, ds, $1, $2, ...)
       │
       ▼
  fyne.Do(table.Refresh())


                         SUBMIT Flow (Client Mode)
                         ═════════════════════════

  Same input → state → button → SUBMIT as above.
  Difference is in the grid handler:

  ┌──────────┐     ┌─────────────────────┐
  │data_grid │     │  Client Mode:       │
  │filter_   │◄────│  filter(masterRows, │
  │mode=     │     │    snapshot)        │
  │"client"  │     │  pure Go, no DB     │
  └────┬─────┘     └─────────────────────┘
       │
       ▼
  fyne.Do(table.Refresh())

  masterRows loaded eagerly during Compose:
  BusinessPool.Query(ctx, MasterDataSource) → masterRows
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `pkg/ui/screen_state.go` | Create | `ScreenState` type with `Set`, `Get`, `Snapshot` |
| `pkg/ui/screen_state_test.go` | Create | Concurrency tests with `-race` flag |
| `pkg/ui/compositor.go` | Modify | Split `Compose` → `composeWithState`; text_input writes to store; button publishes SUBMIT; grid subscribes to `SubmitChannel` with polymorphic dispatch |
| `pkg/ui/compositor_test.go` | Modify | New test scenarios for state convergence, SUBMIT, client-mode filtering |
| `pkg/ui/screen_loader.go` | Modify | `NodeMeta` gains `FilterMode`, `MasterDataSource` JSON fields |
| `pkg/eventbus/eventbus.go` | Modify | Add `SubmitChannel` constant |
| `cmd/golemui/main.go` | Modify | No structural change — `Compose(homeNode)` still called; state creation happens inside `Compose` |
| `docker/init-db/02_init_core.sql` | Modify | Sample vista includes button with `submit_action` and grid with `filter_mode` |

## Interfaces / Contracts

### `pkg/ui/screen_state.go`

```go
package ui

import "sync"

// ScreenState is a thread-safe per-screen state store.
// Inputs write here; buttons read snapshots for SUBMIT.
type ScreenState struct {
    mu    sync.RWMutex
    store map[string]any
}

func NewScreenState() *ScreenState {
    return &ScreenState{store: make(map[string]any)}
}

// Set writes a key-value pair. Called from Fyne OnChanged (UI goroutine).
func (s *ScreenState) Set(key string, value any) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.store[key] = value
}

// Get reads a single key. Returns ("", false) if absent.
func (s *ScreenState) Get(key string) (any, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    val, ok := s.store[key]
    return val, ok
}

// Snapshot returns a defensive copy of the entire store.
// Called from button OnTapped to publish via EventBus.
func (s *ScreenState) Snapshot() map[string]any {
    s.mu.RLock()
    defer s.mu.RUnlock()
    cp := make(map[string]any, len(s.store))
    for k, v := range s.store {
        cp[k] = v
    }
    return cp
}
```

### `pkg/eventbus/eventbus.go` — addition

```go
// SubmitChannel is the fixed channel name for screen SUBMIT events.
const SubmitChannel = "screen:submit"
```

### `pkg/ui/screen_loader.go` — NodeMeta extensions

```go
type NodeMeta struct {
    // ... existing fields ...
    BindTo          string     `json:"bind_to,omitempty"`
    FilterMode      string     `json:"filter_mode,omitempty"`       // "server" (default) | "client"
    MasterDataSource string    `json:"master_data_source,omitempty"` // only for client mode
    // ... existing fields ...
}
```

### `pkg/ui/compositor.go` — key signature changes

```go
// Compose remains the public entry point. Creates state internally.
func Compose(node NodeMeta) (fyne.CanvasObject, error) {
    state := NewScreenState()
    return composeWithState(node, state)
}

// composeWithState is the recursive internal factory.
func composeWithState(node NodeMeta, state *ScreenState) (fyne.CanvasObject, error)
```

### `dataGridModel` extensions

```go
type dataGridModel struct {
    mu              sync.RWMutex
    headers         []string
    columns         []string
    rows            [][]string
    masterRows      [][]string  // eagerly loaded for client-mode
    filterMode      string      // "server" | "client"
    cancel          context.CancelFunc
    unsubscribe     func()
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `ScreenState` Set/Get/Snapshot concurrency | Table-driven + `go test -race`; goroutines writing concurrently, snapshot isolation |
| Unit | `Snapshot` returns defensive copy | Mutate returned map, assert store unchanged |
| Unit | Button with `submit_action` publishes SUBMIT | Inject EventBus, tap button, assert exactly one event on `SubmitChannel` with correct snapshot |
| Unit | Multi-input state convergence | Two inputs write to store, snapshot contains both keys |
| Unit | Server-mode grid receives positional args from snapshot | `trackingMockDBPool` pattern; assert `args` matches snapshot key order |
| Unit | Client-mode grid filters masterRows in-memory | Pre-populate `masterRows`, publish snapshot, assert filtered rows without `BusinessPool.Query` call |
| Unit | Rapid SUBMIT cancels stale server-mode queries | Existing cancel pattern from `TestCompose_DataGrid_ReactiveFiltering` |
| Integration | `Compose` creates state internally and threads it | Full `Compose(container{input+button+grid})` smoke test |
| Regression | All existing tests pass | `go test ./...` — no changes to label, fallback, grid-success, grid-nil-pool tests |

## Migration / Rollout

**Breaking change**: The old `text_input.OnChanged → Publish(bind_to, value)` direct wiring is **removed**. Screens that relied on keystroke-level grid filtering must add a button with `submit_action`. The `data_grid` no longer subscribes to `bind_to` — it only subscribes to `eventbus.SubmitChannel`.

**No database migration**: `FilterMode` and `MasterDataSource` are additive JSONB fields. Absent values default to `FilterMode = "server"` (existing behavior, minus the removed direct wiring). The `docker/init-db/02_init_core.sql` sample vista will be updated to include the new fields.

**Rollback**: Revert commit(s) to restore direct `bind_to` publishing. No schema migration to undo.

**Recommended delivery**: 3 chained PRs per proposal risk assessment:
1. `ScreenState` + `eventbus.SubmitChannel` + `Compose` threading + `text_input`/`button` semantics
2. `data_grid` subscription refactor (subscribe to `SubmitChannel`, server-mode positional args)
3. Client-mode filtering + `masterRows` + SQL seed update

## Open Questions

- [ ] Key ordering for server-mode positional args: snapshot `map[string]any` iteration is non-deterministic. Need a defined key ordering strategy (e.g., alphabetical sort of keys for `$1, $2, …` mapping, or explicit `filter_keys` field on NodeMeta).
- [ ] Client-mode filter matching semantics: exact string match vs substring/contains vs LIKE-pattern. Needs spec clarification.
- [ ] Whether `Compose` should accept an optional `*ScreenState` parameter for test injection, or always create internally (current design creates internally, tests call `composeWithState` directly).
