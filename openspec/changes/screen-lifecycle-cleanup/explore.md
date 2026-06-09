# SDD Explore — screen-lifecycle-cleanup

> Mapping the current state of `EventBus` subscriptions, their lifecycle, and the memory leak from missing cleanup when `ui.Navigate` replaces the active screen. Scope: `pkg/eventbus/eventbus.go`, `pkg/ui/compositor.go`, `cmd/golemui/main.go`, `pkg/ui/screen_state.go`, and the related test files.

## 1. Summary of findings (TL;DR)

GolemUI **does not clean up EventBus subscribers when a screen is replaced**, and the missing cleanup causes a multi-resource memory leak per navigation:

- `EventBus.Subscribe` **already returns a subscription ID** (a `string`) and the interface **already has `Unsubscribe(channel, subID string)`** — the primitive is in place. The leak is purely a *call-site* problem: the data_grid case in `compositor.go` captures the unsubscribe func in `model.unsubscribe` but never invokes it.
- `main.go`'s `ui.Navigate` (the post-`fyne-thread-safety` async version) **only swaps `mainContainer.Objects`** and never calls a teardown on the previous screen. Every `data_grid` on the previous screen therefore leaks: its `EventBus` subscriber, its `dataGridModel`, the captured `*widget.Table`, `*ScreenState`, and `NodeMeta` (all pinned by the closure stored in the `subscribers` map), the in-flight `loadMasterBuffer` / `fetchGridDataAsync` goroutines, and the `context.CancelFunc` stored in `model.cancel`.
- Each `data_grid` instance adds **one** `Subscribe` call. A screen with *N* `data_grid`s therefore leaks *N* subscriber closures (and everything they capture) every time the user navigates away from it. Re-visiting a screen also re-subscribes, so the leak compounds across navigations.
- There is no test that asserts cleanup, no `Compose` teardown, and no `Unsubscribe` call outside the unused `model.unsubscribe` storage. The existing `TestEventBus_Unsubscribe` (eventbus_test.go:54) only tests the bus primitive in isolation, never the compositor.
- The `LocalEventBus` is a **process-wide singleton** (one per `RunBootstrap` call). Its `subscribers` map grows monotonically for the lifetime of the application — no expiration, no LRU, no per-screen teardown.

---

## 2. `pkg/eventbus/eventbus.go` — full file map

File length: 82 lines. Read in full.

### 2.1 Public surface (lines 1-25)

```go
// pkg/eventbus/eventbus.go:1-25
package eventbus

import (
    "fmt"
    "sync"
)

// SubmitChannelPrefix is the prefix for scoped submit channels.
// Actual channels are: "screen:submit:<vistaID>"
const SubmitChannelPrefix = "screen:submit"

type Event struct {
    Channel string
    Payload interface{}
}

type Handler func(Event)

type EventBus interface {
    Publish(channel string, payload interface{})
    Subscribe(channel string, h Handler) string // Returns unique sub ID
    Unsubscribe(channel string, subID string)
}
```

**Critical facts:**
- `EventBus` interface **already has** `Subscribe → string` and `Unsubscribe(channel, subID)`. No API change is required to fix the leak; the only missing piece is the call site.
- `SubmitChannelPrefix = "screen:submit"` is a *prefix*; actual per-screen channels are `fmt.Sprintf("screen:submit:%s", vistaID)` (see `pkg/ui/screen_state.go:23`).

### 2.2 `InMemEventBus` struct (lines 27-32)

```go
// pkg/eventbus/eventbus.go:27-32
type InMemEventBus struct {
    mu          sync.RWMutex
    subscribers map[string]map[string]Handler
    nextSubID   uint64
}
```

- Storage shape: `map[channel]map[subID]Handler`. Outer key is the channel string, inner key is the unique `subID` returned to the caller, inner value is the `Handler` closure.
- **No public accessor for "count subscribers per channel" or "list subscribers"**. The map is package-private. Any cleanup strategy that needs to enumerate subscribers of a channel has two options: (a) add a package-private helper, or (b) add a public method like `UnsubscribeAll(channel string)` / `ChannelStats()` to the interface. Option (a) is sufficient for the compositor's use case; (b) is only needed for diagnostics.

### 2.3 `Subscribe` (lines 39-50)

```go
// pkg/eventbus/eventbus.go:39-50
func (b *InMemEventBus) Subscribe(channel string, h Handler) string {
    b.mu.Lock()
    defer b.mu.Unlock()

    if _, exists := b.subscribers[channel]; !exists {
        b.subscribers[channel] = make(map[string]Handler)
    }

    b.nextSubID++
    subID := fmt.Sprintf("%s:%d", channel, b.nextSubID)
    b.subscribers[channel][subID] = h
    return subID
}
```

- SubID format: `"<channel>:<monotonic uint64>"`. Guaranteed unique across the bus lifetime (monotonic counter, never reset).
- The closure is **stored by value** in the map. Any captured state (model, table, state, node) is **pinned in the map** until the entry is deleted. This is the source of the per-screen leak.

### 2.4 `Unsubscribe` (lines 52-61)

```go
// pkg/eventbus/eventbus.go:52-61
func (b *InMemEventBus) Unsubscribe(channel string, subID string) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if subs, exists := b.subscribers[channel]; exists {
        delete(subs, subID)
        if len(subs) == 0 {
            delete(b.subscribers, channel)
        }
    }
}
```

- Removes the single (channel, subID) entry.
- **If the inner map becomes empty, the outer channel key is also removed.** This is a useful side effect: a screen's `screen:submit:<vistaID>` channel will be entirely deleted once the last subscriber unsubscribes, releasing the inner map.
- Already concurrency-safe.

### 2.5 `Publish` (lines 63-82)

```go
// pkg/eventbus/eventbus.go:63-82
func (b *InMemEventBus) Publish(channel string, payload interface{}) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    subs, exists := b.subscribers[channel]
    if !exists {
        return
    }

    event := Event{
        Channel: channel,
        Payload: payload,
    }

    for _, handler := range subs {
        h := handler
        go h(event)
    }
}
```

- `Publish` dispatches **every handler in its own `go h(event)`**. That is, when a button publishes to `screen:submit:<vistaID>` (compositor.go:137), every active `data_grid` on that screen spawns a new goroutine to run its subscriber handler.
- This means **every publish on a leaked screen's submit channel still spawns goroutines that run on detached Fyne widgets** — even after navigation. In practice the *next* `Navigate` to a different screen publishes on a different channel, so the old screen's leaked subscribers stay dormant. But if the user ever returns to the old vistaID and publishes, the old (now-stale) closures also run.
- Note: a `Publish` on a non-existent channel is a no-op (line 65-67) — so a leak does not crash, it just grows the map.

### 2.6 `NewEventBus` (lines 34-37)

```go
// pkg/eventbus/eventbus.go:34-37
func NewEventBus() EventBus {
    return &InMemEventBus{
        subscribers: make(map[string]map[string]Handler),
    }
}
```

Returns the `EventBus` interface. Called **once** per `RunBootstrap` in `cmd/golemui/main.go:77` and assigned to `ui.LocalEventBus`.

---

## 3. `pkg/ui/compositor.go` — all EventBus touchpoints

File length: 558 lines. Read in full.

### 3.1 Package-level vars (lines 17-22)

```go
// pkg/ui/compositor.go:17-22
var BusinessPool db.DatabasePool
var CorePool db.DatabasePool
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)
```

- `LocalEventBus` is a **process-wide package var**. There is exactly one bus per Go process, created in `RunBootstrap` (main.go:77) and shared across every screen. This is the substrate on which the leak compounds.
- `Navigate` is also a package var; the `Navigate` function is assigned in `RunBootstrap` (main.go:101).

### 3.2 `dataGridModel` (lines 24-33)

```go
// pkg/ui/compositor.go:24-33
type dataGridModel struct {
    mu            sync.RWMutex
    headers       []string
    columns       []string
    rows          [][]string
    masterHeaders []string
    masterRows    [][]string
    filterKeys    []string
    cancel        context.CancelFunc
    unsubscribe   func()
}
```

- `cancel` (line 32) and `unsubscribe` (line 33) are both fields the model already exposes **for the explicit purpose of cleanup**. They are populated below — and then **never called** from anywhere in the codebase (see §3.6).

### 3.3 Every `Subscribe` / `Unsubscribe` / `Publish` call site (production code)

| # | File:Line | Action | Channel | Notes |
|---|---|---|---|---|
| 1 | `compositor.go:137` | `LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())` | `screen:submit:<vistaID>` | Inside a `widget.Button` callback — fires on every "search"/submit button tap. |
| 2 | `compositor.go:216` | `subID := LocalEventBus.Subscribe(state.SubmitChannel(), func(ev eventbus.Event) { ... })` | `screen:submit:<vistaID>` | Inside the `data_grid` case of `composeWithState`. **Only `Subscribe` call site in the codebase.** |
| 3 | `compositor.go:260` | `LocalEventBus.Unsubscribe(state.SubmitChannel(), subID)` | `screen:submit:<vistaID>` | Body of the closure stored in `model.unsubscribe`. Never invoked. |
| 4 | `compositor.go:280` | `LocalEventBus.Publish("publish_selection", rowMap)` | `publish_selection` (global) | Inside `table.OnSelected` — fires on Fyne row selection. Not subscribed to by the compositor; consumers live elsewhere (sidebar / external). Out of scope for this SDD. |

There is **exactly one** `Subscribe` call site (line 216) and **exactly one** matching `Unsubscribe` call site (line 260). The unsubscribe is dead code.

### 3.4 Subscribe count per `Compose` call

Each `Compose(node, vistaID)` invocation (main.go:108, main.go:130) recurses through `composeWithState`. Only the `data_grid` case calls `Subscribe`. Therefore:

- **Subscribes per `Compose` = number of `data_grid` nodes in the screen's layout tree** (any depth, since `composeWithState` recurses through `container` children).
- For a screen with **0** data_grids: 0 subscribes.
- For a screen with **N** data_grids: **N** subscribes — all to the same `screen:submit:<vistaID>` channel, each with a distinct `subID`.

### 3.5 Subscribe callback captured state (lines 216-258)

The closure passed to `Subscribe` captures the following, all by reference:

| Captured name | What it pins |
|---|---|
| `node` (the `NodeMeta` for the current data_grid) | Layout config (filter mode, data source, filter keys, master data source). |
| `model` (`*dataGridModel`) | The mutable state of the grid, including `model.cancel`. |
| `table` (`*widget.Table`) | The Fyne widget. |
| `state` (`*ScreenState`) | The screen's input state map. |
| `LocalEventBus` (package var) | Reference to the bus itself. |
| `filterMasterRows` (function) | Static function reference. |

All of these are pinned in the `subscribers` map as long as the subscriber exists. The `dataGridModel` itself, the `*widget.Table`, the `*ScreenState`, and the `NodeMeta` are the most memory-significant captured values.

### 3.6 What `model.unsubscribe` and `model.cancel` actually do today

`model.unsubscribe` is assigned at lines 258-261:

```go
// pkg/ui/compositor.go:256-262
model.mu.Lock()
model.unsubscribe = func() {
    LocalEventBus.Unsubscribe(state.SubmitChannel(), subID)
}
model.mu.Unlock()
```

`model.cancel` is assigned twice:

- `compositor.go:198-199` — initial context for the eager load:
  ```go
  ctx, cancel := context.WithCancel(context.Background())
  model.cancel = cancel
  ```
- `compositor.go:248-252` — re-assigned inside the subscriber handler when a new submit arrives:
  ```go
  if model.cancel != nil {
      model.cancel()
  }
  subCtx, subCancel := context.WithCancel(context.Background())
  model.cancel = subCancel
  ```

`model.cancel` is therefore **only called from inside the subscriber handler** (compositor.go:249) when a *new* submit happens. It is **never** called from `ui.Navigate` when the screen is replaced.

`grep -rn "model\.unsubscribe\|model\.cancel()" /src/GolemUI/` confirms that `model.unsubscribe` is assigned once and **never invoked** anywhere; `model.cancel()` is invoked once (line 249, inside the subscriber) and also **never invoked** during screen replacement.

### 3.7 Background goroutines tied to the screen lifecycle

The `data_grid` case also kicks off two background goroutines that are tied to the `ctx` stored in `model.cancel`:

1. **`loadMasterBuffer` goroutine** (compositor.go:321) — spawned for `FilterMode == "client"` with a non-empty `MasterDataSource`. Runs `BusinessPool.Query(ctx, ...)`, writes the result into `model.masterHeaders` / `model.masterRows`, then calls `table.Refresh()`. Cancelled by `ctx` only.
2. **`fetchGridDataAsync` goroutine** (compositor.go:480) — spawned for `DataSource != ""` and either from the initial compose or from the submit handler. Runs `BusinessPool.Query(ctx, ...)`, writes the result, then `table.Refresh()`. Cancelled by `ctx` only.

When the screen is replaced:
- The previous `dataGridModel.cancel` is **never called**, so the `ctx` for these goroutines is never cancelled.
- The goroutines continue to completion (or until `BusinessPool.Query` returns). On a long-running master-buffer query, this can keep the goroutine alive for seconds.
- After completion they call `table.Refresh()` and `table.SetColumnWidth()` on a Fyne widget that is no longer in the window tree. With the `fyne-thread-safety` fix in place, these are `fyne.Do` wrapped (per the `fyne-thread-safety` change's spec), so they are not racy — but they still execute pointlessly and the `table.Refresh()` walks a detached Fyne widget tree, which is wasteful.
- The goroutine holds references to the `model`, `table`, `node`, and `state` for its entire lifetime, so the leak also extends to those.

---

## 4. `cmd/golemui/main.go` — `ui.Navigate` and the screen swap

File length: 180 lines. Read in full.

### 4.1 `LocalEventBus` initialization (lines 76-78)

```go
// cmd/golemui/main.go:75-78
// 3. Event bus setup (pkg/eventbus)
eb := eventbus.NewEventBus()
ui.LocalEventBus = eb
```

- Created **once** during `RunBootstrap`. The returned `eb` is also stored on the `App` struct (main.go:143) and exposed as `App.EventBus`. The compositor only ever sees the package var `ui.LocalEventBus`.
- There is no per-screen bus, no channel scoping beyond the `screen:submit:<vistaID>` string convention, and no `Close()` / `Shutdown()` method on the bus (none exists in the package).

### 4.2 `ui.Navigate` — full body (lines 100-116)

```go
// cmd/golemui/main.go:100-116
// Setup navigation callback — updates only the right panel
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    node, err := ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)
    if err != nil {
        log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
        return
    }
    newUI, err := ui.Compose(node, vID)
    if err != nil {
        log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
        return
    }
    mainContainer.Objects = []fyne.CanvasObject{newUI}
    mainContainer.Refresh()
    navTree.SelectByVistaID(vID)
}
```

This is the **post-`fyne-thread-safety` async-version** (the `LoadScreen` + `Compose` run inside a `go func()`; only the final UI swap runs on the UI thread via `fyne.Do` — see the prior change's explore.md for the wrap pattern). What matters for this SDD:

- The closure captures `ctx`, `cfg.LayoutQuery`, `ui.CorePool`, `mainContainer`, and `navTree`.
- The *previous* `mainContainer.Objects[0]` (the previous screen's `fyne.CanvasObject`) is **replaced** by `mainContainer.Objects = []fyne.CanvasObject{newUI}` (line 113). The previous object becomes unreachable from the UI tree.
- **No teardown hook is called on the previous screen.** There is no:
  - `LocalEventBus.UnsubscribeAll("screen:submit:<oldVistaID>")`
  - walk of the previous `NodeMeta` tree to collect subscription IDs
  - call to `model.cancel` / `model.unsubscribe` on the previous data_grids
  - signal to a per-screen context to terminate in-flight goroutines
- The old screen's resources therefore leak into the `LocalEventBus.subscribers` map and into the in-flight `loadMasterBuffer` / `fetchGridDataAsync` goroutines.

### 4.3 Other screen-swap paths

| Path | Location | Cleanup? |
|---|---|---|
| Initial bootstrap | `main.go:128-138` (compose `homeUI`, set as `mainContainer.Objects[0]`, then `win.SetContent(split)`) | N/A — first screen, no previous state. |
| `ui.Navigate(vID)` (from a button with `SubmitAction: "navigate:..."`) | `main.go:100-116` (called from the button's `OnTapped` closure, compositor.go:131-133) | **No cleanup** — this is the leak. |
| `ui.Navigate` from sidebar tree selection | (not in the current main.go; only the button path exists today) | N/A — does not exist yet. |

---

## 5. `pkg/ui/screen_state.go` — screen-scoped state and submit channel

File length: 60 lines. Read in full. Critical for understanding the leak:

- `ScreenState` is a per-screen struct created in `Compose(node, vistaID)` (compositor.go:62 → `NewScreenState(vistaID)`).
- The submit channel is `fmt.Sprintf("screen:submit:%s", vistaID)` (screen_state.go:23). **One channel per screen**, shared by every data_grid on that screen.
- The submit channel is a *string key* into the bus, not a Go `chan`. Cleanup therefore is by channel-string + subID.
- `ScreenState` is captured by the `data_grid` subscriber closure at compositor.go:216 (via the outer closure over `state *ScreenState`). Every `data_grid` on the screen keeps a pointer to the same `ScreenState`. The `ScreenState` is therefore pinned in the bus's `subscribers` map for the duration of the leak.

---

## 6. Existing tests

### 6.1 `pkg/eventbus/eventbus_test.go` (160 lines)

| Test | Lines | What it covers |
|---|---|---|
| `TestEventBus_HappyPath` | 13-50 | Subscribe + Publish + handler receives event. Uses `sync.WaitGroup` with a 1s timeout. |
| `TestEventBus_Unsubscribe` | 54-67 | Subscribe → Unsubscribe → Publish → handler should not fire. **Tests the bus primitive only**, in isolation. No test of the compositor's use of it. |
| `TestEventBus_SlowSubscriber` | 71-122 | Confirms that Publish's `go h(event)` fan-out does not serialize handlers (a slow subscriber doesn't block a fast one). |
| `TestEventBus_Concurrency` | 126-176 | 100 goroutines, parallel Subscribe / Publish / Unsubscribe. No leak assertion. |
| `TestSubmitChannelPrefix_Constant` | 178-182 | Guards the constant string. |

**What is missing:** there is no test that asserts `Unsubscribe` is called when a screen is replaced; no test that asserts subscriber count goes back to 0 after navigation; no test that asserts the closure's captured state is GC-eligible after teardown.

### 6.2 `pkg/ui/compositor_test.go` — EventBus-related test patterns

A `grep -n "Subscribe\|Unsubscribe\|Publish\|EventBus"` over the file (35KB) finds 10 sites:

| Line | Test | Pattern |
|---|---|---|
| 320-322 | unnamed data_grid test | `eb := eventbus.NewEventBus(); ui.LocalEventBus = eb; defer func() { ui.LocalEventBus = nil }()` — the standard test fixture. |
| 468-498 | `TestCompose_TextInput_WritesToState_NoPublish` | Subscribes to a test channel, asserts text_input does NOT publish to it. |
| 504-540 | unnamed input test | Subscribes to a generic channel to assert no spurious publishes. |
| 538-547 | `TestCompose_Button_SubmitAction_PublishesSnapshot` | Subscribes to `screen:submit:test-vista`, asserts the button's `OnTapped` publishes a snapshot there. |
| 622-628 | `TestCompose_Button_NoSubmitAction_NoPublish` | Same pattern, negative case. |
| 669-671 | unnamed data_grid test | Same fixture. |
| 774-776 | unnamed data_grid test | Same fixture. |
| 877-893 | unnamed data_grid test | Subscribes to `screen:submit:screen-a` and `screen:submit:screen-b` simultaneously to assert channel scoping. |
| 968-970 | unnamed data_grid test | Same fixture. |
| 1062-1064 | unnamed data_grid test | Same fixture. |
| 1175-1184 | `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` | Subscribes to `publish_selection` and asserts the row map arrives. |
| 1261-1267 | `TestCompose_DataGrid_RowSelection_OutOfBounds_NoPublish` | Same channel, negative case. |
| 1321-1322 | `TestCompose_DataGrid_RowSelection_NilEventBus_NoPanic` | Sets `ui.LocalEventBus = nil` and asserts no panic. |

**No test invokes `Unsubscribe` from a teardown.** The `defer func() { ui.LocalEventBus = nil }()` pattern resets the global var but does not exercise any cleanup path inside the compositor. The `TestCompose_ButtonNavigation` test (compositor_test.go:1368-1399) swaps in a fake `ui.Navigate` and `test.Tap(btn)` but never asserts on subscriber lifecycle.

---

## 7. Answers to the 6 questions

### Q1. What is the current `Subscribe` return type? Does it return an unsubscribe func, a subscription ID, or nothing?

**Subscription ID (`string`)** — see `pkg/eventbus/eventbus.go:24` (`Subscribe(channel string, h Handler) string // Returns unique sub ID`) and the implementation at lines 39-50. The `subID` is `fmt.Sprintf("%s:%d", channel, b.nextSubID)`.

**Not** an unsubscribe func. The compositor currently has to *manually wrap* the subID in a closure (`compositor.go:259-261`) to produce its own `model.unsubscribe` func.

### Q2. How many `Subscribe` calls happen per screen composition?

**Exactly one `Subscribe` call site** in the entire codebase (`pkg/ui/compositor.go:216`). Per `Compose` invocation, it is called **once per `data_grid` node** in the screen's layout tree (any depth). Every non-data_grid component does not subscribe.

So:
- A screen with 0 data_grids → 0 subscribes.
- A screen with 1 data_grid → 1 subscribe.
- A screen with N data_grids → N subscribes, all to the same `screen:submit:<vistaID>` channel, each with a distinct subID.

There are no other `Subscribe` callers in the project (no plugins, no sidebar widget, no loader).

### Q3. What happens to the old screen's subscribers when `Navigate` replaces the UI?

**They leak.** Specifically, the data_grid case stores `model.unsubscribe` on the `dataGridModel`, but `ui.Navigate` (main.go:100-116) does not walk the previous screen's widget tree, does not access the previous `dataGridModel`s, does not call `LocalEventBus.Unsubscribe` on any subID, and does not cancel `model.cancel`. The old `screen:submit:<oldVistaID>` channel and all of its subscribers remain in `LocalEventBus.subscribers` forever.

Captured by those subscribers and therefore also leaked for the process lifetime:
- The `dataGridModel` struct (and its `rows`, `masterRows`, `headers`, `columns`, `masterHeaders`, `filterKeys` slices).
- The `*widget.Table` Fyne widget.
- The `*ScreenState` and its `data` map.
- The `NodeMeta` for the data_grid.

In addition, in-flight `loadMasterBuffer` and `fetchGridDataAsync` goroutines for the old screen keep running because `model.cancel` is never called. They eventually finish and call `table.Refresh()` on a detached widget.

### Q4. Is there an `Unsubscribe` method already on the `EventBus` interface?

**Yes** — `pkg/eventbus/eventbus.go:24`:

```go
Unsubscribe(channel string, subID string)
```

Implemented in `InMemEventBus.Unsubscribe` (lines 52-61). Concurrency-safe. The bus primitive is sufficient; no API change is needed.

### Q5. What data structure stores subscribers? Is there a way to count active subscribers per channel?

**Storage:** `map[string]map[string]Handler` (channel → subID → handler), guarded by `sync.RWMutex`. Defined at `pkg/eventbus/eventbus.go:28-31`.

**Counting:** **No** public method exposes the count, and the map is package-private. To count subscribers per channel you would need to either:
- add a public method like `Stats() map[string]int` or `SubscriberCount(channel string) int` to the `EventBus` interface, or
- add a package-private helper in `pkg/eventbus` (e.g. `func (b *InMemEventBus) channelSize(channel string) int`) callable from a test in the same package.

The simplest fit-for-purpose addition for testing the leak is a package-private test-only helper. A public method is only warranted if production code needs it (e.g. for diagnostics or a periodic sweep).

### Q6. Are there any other resources (goroutines, channels, timers) tied to the screen lifecycle that also leak?

**Yes, at least three more classes of resource** beyond the EventBus subscribers:

1. **In-flight `loadMasterBuffer` goroutines** (`compositor.go:321`). Spawned for client-mode data_grids. Run to completion, then call `table.Refresh()` on a detached widget. Hold references to `model`, `table`, `node`, and `state` for the duration of the query.

2. **In-flight `fetchGridDataAsync` goroutines** (`compositor.go:480`). Spawned for any data_grid with a non-empty `DataSource` (server-mode or client-mode submit handler). Same leak shape as (1).

3. **The `model.cancel` `context.CancelFunc`** (`compositor.go:32, 199, 252`). Stores a `context.WithCancel` handle for the *current* eager/submit load. Cancelled on the next submit (compositor.go:249), but **never** on screen replacement, so the (1) and (2) goroutines keep running.

The chained dependency is: `ui.Navigate` is the *only* place where the leak can be plugged, because it is the *only* place that knows the previous screen is being discarded. The bus alone cannot tell the difference between a "stale" subscriber and a "still-wanted" one. Conversely, walking the widget tree is unnecessary if `Compose` returns a *teardown function* alongside the widget — the simplest fix is to make `Compose` return `(fyne.CanvasObject, func(), error)`.

No `time.Timer` or `time.Ticker` is used in the compositor, so there are no timer leaks.

---

## 8. Files retrieved

1. `pkg/eventbus/eventbus.go` (lines 1-82) — full file, the bus primitive (Subscribe returns subID, Unsubscribe exists, no diagnostics, `go h(event)` per Publish).
2. `pkg/ui/compositor.go` (lines 1-558) — full file, every EventBus touchpoint, `dataGridModel` (with dead `cancel` and `unsubscribe` fields), `data_grid` case at lines 184-285, `Subscribe` at line 216, `Unsubscribe` at line 260 (dead), `Publish` calls at lines 137 and 280.
3. `cmd/golemui/main.go` (lines 1-180) — full file, `LocalEventBus` init at line 77, async `ui.Navigate` at lines 100-116, `mainContainer.Objects` swap at line 113, no teardown.
4. `pkg/ui/screen_state.go` (lines 1-60) — full file, per-screen `ScreenState` with `screen:submit:<vistaID>` channel.
5. `pkg/eventbus/eventbus_test.go` (lines 1-182) — full file, existing tests cover the bus primitive in isolation; `TestEventBus_Unsubscribe` at line 54.
6. `pkg/ui/compositor_test.go` — surveyed via grep: 10+ tests use the `eb := eventbus.NewEventBus(); ui.LocalEventBus = eb; defer func() { ui.LocalEventBus = nil }()` fixture; none assert subscriber cleanup. `TestCompose_ButtonNavigation` at lines 1368-1399 uses a fake `ui.Navigate` without a teardown assertion.
7. `openspec/changes/fyne-thread-safety/explore.md` (read for format template and to confirm the `fyne-thread-safety` async-Navigate pattern is already in place).
8. `openspec/specs/` (listed via `ls`) — no existing spec explicitly covers screen-lifecycle cleanup. The closest spec, `reactive-input-publishing` / `client-reactivity-broker`, describes *how* subscribers are created but not how they are torn down.

## 9. Gap between current behavior and a correct screen-lifecycle

| Required behavior | Current state | Gap |
|---|---|---|
| `EventBus` has `Subscribe` → `string` and `Unsubscribe(channel, subID)` | ✅ Already present (eventbus.go:24, 52-61) | None — primitive is sufficient. |
| `Compose` exposes a teardown hook (or returns a cleanup func) | ❌ `Compose` returns `(fyne.CanvasObject, error)` only (compositor.go:62) | Need to plumb cleanup. Two viable shapes: (a) `Compose` returns `(fyne.CanvasObject, func(), error)`; (b) `ScreenState` / `dataGridModel` exposes a `Close()` / `Dispose()` and `ui.Navigate` walks the tree to call it. |
| `ui.Navigate` calls teardown on the previous screen before composing the new one | ❌ `ui.Navigate` only swaps `mainContainer.Objects` (main.go:113) | Need to capture the previous screen's teardown func (from option (a) above) and invoke it before the swap. |
| `dataGridModel.unsubscribe()` and `dataGridModel.cancel()` are invoked on teardown | ❌ Assigned at compositor.go:259 and 199, **never invoked** | Need a single teardown point that calls both, then `nil`-s them. |
| In-flight `loadMasterBuffer` / `fetchGridDataAsync` goroutines terminate on teardown | ❌ `model.cancel` is never called on screen replacement | Calling `model.cancel()` on teardown cancels the `ctx` they observe. |
| Tests assert subscriber count goes back to 0 after teardown | ❌ Not covered | Need (a) a package-private test helper in `pkg/eventbus` to count subscribers per channel, and (b) a `TestCompose_TearsDownSubscribers` / `TestNavigate_TearsDownPreviousScreen` test. |
| The `EventBus` exposes a way to count subscribers per channel (for diagnostics) | ❌ No method exists | Optional. Useful for the verification test. |

The change is therefore **not** a deep refactor — the bus primitive is in place, the call site has the wiring (`model.unsubscribe`, `model.cancel`) but does not invoke it, and the missing piece is a single teardown point that `ui.Navigate` can call. Expected diff size: small (probably 30-80 lines of production code, plus tests).

## 10. Start here

Begin with `pkg/ui/compositor.go:62-65` (the `Compose` signature) and `pkg/ui/compositor.go:198-200` (the `dataGridModel.cancel` field) to decide the teardown API shape, then read `cmd/golemui/main.go:100-116` (the async `ui.Navigate`) to identify the call site that must invoke it. The two production-code anchors are **adding a cleanup func to the `Compose` return tuple** and **calling it in `ui.Navigate` before the `mainContainer.Objects` swap**. The test anchor is **`pkg/ui/compositor_test.go`** — add a test that asserts `LocalEventBus` has zero subscribers for `screen:submit:<oldVistaID>` after navigation.
