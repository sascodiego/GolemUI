## Exploration: screen-state-store

> **Change**: `screen-state-store` — evolve GolemUI from direct input→EventBus→grid reactivity to centralized, per-screen state with a consolidated `SUBMIT` event and polymorphic filtering (`server-side` parameterized vs `client-side` in-memory).
>
> **Mode**: hybrid (engram + openspec). **TDD**: enabled.

---

### Current State

GolemUI today uses a **direct, point-to-point pub/sub** wiring between inputs and grids, implemented entirely in [file:///src/GolemUI/pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go):

1. **`text_input`** (line 90-99) attaches a closure to `entry.OnChanged` that synchronously publishes the new value to `LocalEventBus` under the channel `node.BindTo`. There is no local store — every keystroke is broadcast immediately.
2. **`data_grid`** (line 104-170) subscribes to `node.BindTo` and, on every received event, cancels any in-flight query via a per-model `context.CancelFunc` and re-runs `BusinessPool.Query(ctx, node.DataSource, args...)` where `args` is the *raw event payload* (so positional `$1` consumes a single value).
3. **`button`** (line 101-102) is a `widget.NewButton(node.Label, func() {})` — its `OnTapped` callback is empty. `NodeMeta.SubmitAction` exists in the struct (line 47) but is never read.
4. The screen loader ([file:///src/GolemUI/pkg/ui/screen_loader.go](file:///src/GolemUI/pkg/ui/screen_loader.go)) simply unmarshals `config_columnas` JSONB into `NodeMeta`; no extra metadata.
5. The event bus ([file:///src/GolemUI/pkg/eventbus/eventbus.go](file:///src/GolemUI/pkg/eventbus/eventbus.go)) is an in-memory broker with `Publish(channel, payload interface{})`, `Subscribe(channel, handler) subID`, `Unsubscribe(channel, subID)`. Handlers are invoked in their own goroutine (`go h(event)` on line 75).
6. **No centralized state** exists anywhere in the client. There is no structure that aggregates inputs belonging to the same screen. There is no notion of "current filter map". The data grid filters react only to whatever each individual input publishes.
7. **No client-side mode** for the grid. Every data grid fetches from `BusinessPool` on every event. The bootstrap in [file:///src/GolemUI/cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go) does not preload any master data buffer.

Key technical primitives already present that this change can build on:

- `dataGridModel` ([file:///src/GolemUI/pkg/ui/compositor.go:21-28](file:///src/GolemUI/pkg/ui/compositor.go)) — a struct holding `headers`, `columns`, `rows`, `cancel`, `unsubscribe`, guarded by `sync.RWMutex`. It is *per grid instance*, not per screen.
- `NodeMeta` ([file:///src/GolemUI/pkg/ui/compositor.go:37-51](file:///src/GolemUI/pkg/ui/compositor.go)) — declarative metadata tree with `BindTo`, `SubmitAction`, `DataSource`, `Label`, `Placeholder`, `DefaultValue`, recursive `Children`.
- `MockDBPool` ([file:///src/GolemUI/pkg/db/mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go)) — exact-string query matching, suitable for asserting that grids received `$1`, `$2`, etc.
- Existing tests in [file:///src/GolemUI/pkg/ui/compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go) demonstrate patterns for: wiring `ui.LocalEventBus` + `ui.BusinessPool` from tests, calling `test.Type(entry, "...")` to simulate input, polling `trackingPool.queriesCalled` for assertions, and asserting `ctxErrAtQuery` to verify stale-query cancellation.
- The `EventBus` already supports arbitrary `interface{}` payloads, so a `map[string]any` or typed struct is a natural fit.
- `screen-loading` (in-progress) treats the recursive `Compose(node)` call in `main.go` as the single root that owns a screen — this is the natural seam at which to instantiate a per-screen store.

---

### Affected Areas

| File | Why it is affected |
|---|---|
| [file:///src/GolemUI/pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go) | Core change. `Compose` (a) threads a per-screen `*ScreenState` through recursion; (b) `"text_input"` writes into the store instead of publishing; (c) `"button"` (when `submit_action` is set) publishes a single `SUBMIT` event with the store snapshot as payload; (d) `"data_grid"` subscribes to a fixed SUBMIT channel, then dispatches to either parameterized `BusinessPool.Query(ctx, ds, args...)` (server mode) or an in-memory filter over a master buffer (client mode); (e) `dataGridModel` gains `masterRows [][]string` and a `filterMode` field. |
| `pkg/ui/screen_state.go` (NEW) | Defines the `ScreenState` type: thread-safe `map[string]any` (writes from input goroutines, reads from the SUBMIT handler goroutine), plus `Snapshot() map[string]any` to copy under a read-lock before publishing. Pure data, no Fyne imports. |
| `pkg/ui/screen_state_test.go` (NEW) | Unit tests: concurrent Set/Get, Snapshot returns a defensive copy (mutations to the snapshot do not affect the store), nil safety. |
| [file:///src/GolemUI/pkg/ui/screen_loader.go](file:///src/GolemUI/pkg/ui/screen_loader.go) | The `NodeMeta` JSON contract gains two optional fields: `FilterMode` (`"server"` \| `"client"`, default `"server"`) and `MasterDataSource` (string, only used when `FilterMode == "client"`). Both must deserialize cleanly when absent. |
| [file:///src/GolemUI/cmd/golemui/main.go](file:///src/Golemui/cmd/golemui/main.go) | Bootstrap now (a) creates one `ScreenState` per top-level screen and passes it to `Compose`; (b) if any grid is `client`-mode, eagerly loads its master buffer via `BusinessPool.Query(ctx, masterDataSource)` *before* calling `Compose`, and injects the preloaded rows into the screen state. |
| [file:///src/GolemUI/pkg/ui/compositor_test.go](file:///src/GolemUI/pkg/ui/compositor_test.go) | New tests: multi-input write converges into a single state map; button click publishes one SUBMIT with all keys; server-mode grid receives positional `$1, $2, ...`; client-mode grid filters in-memory without hitting `BusinessPool`; rapid Submit clicks cancel stale queries (existing pattern reused). |
| [file:///src/GolemUI/cmd/golemui/main_test.go](file:///src/Golemui/cmd/golemui/main_test.go) | Extend `TestRunBootstrap_Success` to register a `home` vista that contains a `text_input` + `button` + `data_grid`, and assert that the event bus receives exactly one SUBMIT after a button tap. |
| [file:///src/GolemUI/docker/init-db/02_init_core.sql](file:///src/GolemUI/docker/init-db/02_init_core.sql) | Sample `home` vista row must include two `text_input`s, a `button` with `submit_action="home:submit"`, and a `data_grid` with `data_source` containing `WHERE title LIKE $1 AND amount >= $2` so the team can manually validate the new flow. |
| [file:///src/GolemUI/openspec/config.yaml](file:///src/GolemUI/openspec/config.yaml) | No structural change; the new `screen-state-store` change folder is a peer of `screen-loading-db`. |
| [file:///src/GolemUI/pkg/eventbus/eventbus.go](file:///src/GolemUI/pkg/eventbus/eventbus.go) | **No change required.** The bus already accepts arbitrary payloads and supports fixed channel names. SUBMIT is just a channel name like any other. |

---

### Approaches

#### A1. Per-screen `*ScreenState` threaded through `Compose` (composition root)

```go
func Compose(node NodeMeta) (fyne.CanvasObject, error)             // public, root, creates state
func composeWithState(node NodeMeta, state *ScreenState) (...)     // internal recursive
```

- **Pros**: Explicit ownership; lifetime is tied to the screen; trivially testable by passing a `*ScreenState` directly in unit tests; no global mutation; compatible with multiple windows/tabs in the future.
- **Cons**: Slight API churn — `Compose` keeps its signature, but the recursive `composeWithState` is a new private function. Anyone calling `Compose` from outside `main.go` (e.g. recursive nodes) must switch to the internal variant.

#### A2. Global map `ui.ScreenStates[viewID]*ScreenState`

- **Pros**: No signature change.
- **Cons**: Hidden global state; leaks across screens; must be cleared on screen disposal; not testable in parallel without `t.Cleanup` discipline; violates the "no global mutation" rule of clean architecture.

#### A3. Carry state in `context.Context` parameter

- **Pros**: Idiomatic Go; can be set/retrieved anywhere via helpers.
- **Cons**: `context.WithValue` is famously misused for non-request-scoped data; forces a context parameter through every widget factory; introspection ("does this state have key X?") is awkward.

**Approaches to payload format for SUBMIT**

#### B1. Raw `map[string]any` snapshot

```go
LocalEventBus.Publish("screen:submit", state.Snapshot())
```

- **Pros**: Trivially extensible; no Go struct to maintain; serializes cleanly for cross-process needs later.
- **Cons**: Type-erased on the consumer side; consumer must coerce (`val.(string)`).

#### B2. Typed struct `SubmitPayload{ Filters map[string]any }`

- **Pros**: Discoverable; IDE-friendly; testable via struct equality.
- **Cons**: Slight ceremony; will likely grow fields over time.

**Approaches to the SUBMIT channel name**

#### C1. Fixed constant `eventbus.SubmitChannel = "screen:submit"`

- **Pros**: Simpler mental model — every screen uses the same channel; a button "submits the screen" and every grid on that screen reacts. Matches the user's wording "evento unificado SUBMIT".
- **Cons**: If two screens are visible simultaneously (future), both will receive both screens' SUBMITs; acceptable in single-window MVP.

#### C2. Per-button `submit_action` becomes the channel (e.g. `node.SubmitAction`)

- **Pros**: Reuses an existing field that is already declared on `NodeMeta`; allows routing a single button to a specific grid.
- **Cons**: Diverges from "unified" SUBMIT; inverts the relationship (button names the channel, grid must match).

**Approaches to mode configuration on the grid**

#### D1. New `FilterMode` field on `NodeMeta` (server | client, default server)

- **Pros**: Declarative; lives in the same JSONB that already configures everything; matches GolemUI's "metadata-driven" identity.
- **Cons**: Adds a new JSON contract field (low blast radius — defaults are server, so old screens still work).

#### D2. Per-input `filter_mode` on `text_input` instead of per-grid on `data_grid`

- **Pros**: Could in theory mix modes per filter.
- **Cons**: Conceptually wrong — the *grid* is the one that decides whether to query or filter locally, not the inputs.

**Approaches to master-buffer loading for client-mode grids**

#### E1. Eager, top-of-screen, single `MasterDataSource` on the grid

- The grid declares `master_data_source`; during `Compose` the compositor fires the master query once and stores the rows on the model.

- **Pros**: Declarative; supports any number of client-mode grids; uses the same `BusinessPool` infra.
- **Cons**: Master data is in memory; if the table is huge this changes the memory profile. For MVP with transactional data sets this is acceptable.

#### E2. Lazy — only fetch master when the user first hits SUBMIT

- **Pros**: Faster initial render.
- **Cons**: First SUBMIT is slow; defeats the "load once" promise in the requirements.

---

### Recommendation

Adopt **A1 + B1 + C1 + D1 + E1**.

- **`A1`** — Thread a per-screen `*ScreenState` through `Compose`. The recursive factory takes `state` as a parameter. The public `Compose` (called by `main.go`) constructs the state. This is the cleanest ownership model and the easiest to test in isolation.
- **`B1`** — Publish a `map[string]any` snapshot. It is the most flexible payload for polymorphic downstream consumers (server-mode grid reads it positionally, client-mode grid filters over its buffer). A typed wrapper can be added later if needed without breaking the channel.
- **`C1`** — Use a single fixed channel `eventbus.SubmitChannel` (constant `"screen:submit"`). The "unified" wording in the requirements makes this the natural choice. In the single-window MVP there is no cross-screen interference; when multi-window support lands, the `*ScreenState` already carries the scope.
- **`D1`** — Add `FilterMode` to `NodeMeta` with values `"server"` (default) and `"client"`. Adding `MasterDataSource` only for client-mode. The `NodeMeta` JSON contract stays backwards-compatible (new fields are optional).
- **`E1`** — When `FilterMode == "client"`, the compositor eagerly runs `BusinessPool.Query(ctx, MasterDataSource)` once during `Compose` and stores the rows on the `dataGridModel.masterRows`. SUBMIT events then run a pure-Go row filter against that buffer.

The composite behavior for server mode is exactly the current `fetchGridDataAsync(ctx, ds, model, table, args...)` flow — re-use the existing `dataGridModel`, `context.CancelFunc`, and `fyne.Do(table.Refresh())` machinery. The cancel-on-rapid-Submit pattern already proven in `TestCompose_DataGrid_ReactiveFiltering` carries over unchanged.

For client mode, the same handler runs a pure filter: `rows = filter(masterRows, state)` under the model's `mu.Lock()`, then `fyne.Do(table.Refresh())`. No `BusinessPool.Query` is ever called, satisfying the "manipulación en memoria solo cuando el modo esté explícitamente configurado" rule.

The button semantics become:

```go
case "button":
    if node.SubmitAction != "" {
        return widget.NewButton(node.Label, func() {
            LocalEventBus.Publish(eventbus.SubmitChannel, state.Snapshot())
        }), nil
    }
    return widget.NewButton(node.Label, func() {}), nil
```

`eventbus.SubmitChannel` is the constant introduced in this change; declaring it on the `eventbus` package keeps cross-package coupling obvious.

---

### Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| **Breaking the existing reactive-input-publishing spec.** Today `text_input` publishes to `bind_to` directly. After this change, a `text_input` with `bind_to` *and no button* on the screen will silently stop notifying the grid. | Medium | Detect at compose time: if any grid on the screen uses `bind_to` for filter input, the change must remain compatible OR the spec is explicitly evolved. The proposal phase will demote the old `bind_to`-based grid subscription to "deprecated, supported only in server-mode when state is empty" or simply remove it. Prefer explicit removal + release note to keep the model clean. |
| **400-line PR budget breach.** This change touches `compositor.go`, adds `screen_state.go`, modifies `screen_loader.go` (NodeMeta struct), updates tests, and updates SQL seed data. Realistic diff: 350-500 lines. | Medium-High | The `sdd-tasks` phase MUST forecast this. Split work: (1) introduce `ScreenState` + wire it through `Compose`; (2) add SUBMIT button + refactor data_grid subscription; (3) implement client-mode filtering + master-data bootstrap. Each is a reviewable slice. |
| **Concurrency in `ScreenState`.** Inputs can write from arbitrary goroutines (Fyne's onChanged runs on the UI goroutine, but the SUBMIT handler runs on the EventBus goroutine). | Low | `ScreenState` is protected by `sync.RWMutex`; `Snapshot()` returns a copy under RLock. Add a race-detector test in `screen_state_test.go` (`go test -race`). |
| **Master data memory cost.** A 100k-row table held in memory per client-mode grid. | Low for MVP | Document the limit; defer streaming/pagination to a future change. The user's spec explicitly says "búfer maestro en memoria". |
| **Order of SUBMIT payload vs grid subscription.** If a grid is composed *before* a button publishes (which is the normal case), subscription is in place. But if SUBMIT fires during composition (impossible in current Fyne flow, but worth a test), the grid may miss it. | Low | `Compose` is synchronous; EventBus publishes happen only after user interaction. Add a defensive test that no SUBMIT occurs during `Compose`. |
| **`MockDBPool` exact-string matching for parameterized queries.** The new tests must register queries with the exact `$1, $2, ...` placeholder count. | Low | Reuse the pattern from `TestCompose_DataGrid_ReactiveFiltering`; assertion will check `len(call.args)` matches the number of filters in the state. |
| **Backwards compatibility with the home vista in `02_init_core.sql`.** Existing rows do not declare `FilterMode`; defaulting to `"server"` keeps them working. | Low | Default the field in Go (`FilterMode == ""` → `"server"`) AND in the SQL seed when explicitly setting client mode. |
| **Thread-safety in the recursive `composeWithState`.** Each input/grid receives a pointer to the same `*ScreenState`; the closures they register on Fyne widgets may outlive the screen. | Low | Mirror the existing `unsubscribe` pattern in `dataGridModel`: when a `*ScreenState` is destroyed, all its subscribers must be torn down. For MVP, screen lifetime == app lifetime, so this is moot. Document for future. |

---

### Ready for Proposal

**Yes.** The investigation is complete:

- All three mandatory files have been read in full and cross-referenced with the existing reactive-input-publishing and parametrized-grid-filtering specs.
- The four primary design decisions (state lifecycle, payload format, channel name, mode configuration) have been enumerated and a defensible combination has been chosen.
- The change is large enough that `sdd-tasks` must forecast the 400-line budget and propose a chained-PR slicing.
- The minimal first PR slice is well-defined: introduce `ScreenState` + `eventbus.SubmitChannel` + refactor `Compose` to thread state + change `text_input`/`button` semantics. This alone is testable and is the smallest change that establishes the new pattern. The data_grid polymorphic mode can ship in a second slice.

The orchestrator can now hand off to `sdd-propose` for the change named `screen-state-store`.
