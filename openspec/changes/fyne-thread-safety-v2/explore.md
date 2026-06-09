# SDD Explore Report — fyne-thread-safety-v2

> Mapping the CURRENT thread-safety state of GolemUI's Fyne UI code, four weeks after the v1 change (`fyne-thread-safety`, PR #23) was merged and verified. Scope per spec `docs/specify/016-fyne-thread-safety-concurrency.md`: `cmd/golemui/main.go`, `pkg/ui/sidebar_widget.go`, `pkg/ui/compositor.go`, `pkg/eventbus/eventbus.go` (plus the relevant test files).

## 1. Executive Summary

The v1 change (`openspec/changes/fyne-thread-safety`) was **substantially regressed** by two subsequent refactors. The original `fyne.Do(...)` wraps for table mutations in `compositor.go` and the `fyne.Do(...)` wrap around the `ui.Navigate` UI swap in `main.go` no longer exist in the code. They were replaced — not augmented — by:

* A `refreshMu sync.Mutex` on `dataGridModel` (`compositor.go:26`) used to **serialize** widget mutations from competing goroutines, instead of dispatching them onto the Fyne UI thread via `fyne.Do`. This is a different mechanism and **does not satisfy the Fyne threading contract**.
* A `cleanupMu sync.Mutex` in `main.go:107` for `prevCleanup` bookkeeping. This addresses cleanup races, not UI-thread dispatch.

**Result:** every `fyne.Do` wrap from v1 is gone. The current code has **zero** `fyne.Do` / `fyne.DoAndWait` calls in production (`grep -rn "fyne.Do" --include="*.go"` returns only test comments). All of the following UI mutations are called from background goroutines with no UI-thread dispatch:

1. `compositor.go:371, 373` — `table.SetColumnWidth` loop + `table.Refresh()` inside the `loadMasterBuffer` goroutine.
2. `compositor.go:399` — `table.Refresh()` inside `filterMasterRows` empty-snap path (called from the event-bus subscriber handler goroutine).
3. `compositor.go:437` — `table.Refresh()` inside `filterMasterRows` filtered path (same call chain).
4. `compositor.go:519, 521` — `table.SetColumnWidth` loop + `table.Refresh()` inside the `fetchGridDataAsync` goroutine.
5. `main.go:134-137` — `mainContainer.Objects` assignment + `mainContainer.Refresh()` + `navTree.SelectByVistaID(vID)` inside the `ui.Navigate` goroutine.
6. `sidebar_widget.go:52, 57` — `nt.tree.OpenBranch` (in a `for` loop) + `nt.tree.Select`, called transitively from the `ui.Navigate` goroutine via `navTree.SelectByVistaID(vID)`.

Net for v2: the four spec areas (Navigate, Sidebar, DataGrid, EventBus) are all still open. v1's `fyne.Do` work has to be re-applied (the `refreshMu` mutex is **not** a substitute — see §3.3), and two new areas must be covered: `sidebar_widget.go:SelectByVistaID` and the EventBus subscriber handler's UI mutations (which the v2 spec broadens in anticipation of the `017` and `018` reactive-binding changes).

**Note on the verification artefacts from v1:** the v1 verify-report at `openspec/changes/fyne-thread-safety/verify-report.md:50-58` and apply-report at `apply-report.md:55-77` were *correct at the time* but describe code that no longer exists. The git history confirms the regression: v1 wraps in `compositor.go` were removed by `abc18d6` ("refactor(ui): remove direct DB access from compositor") and the wrap in `main.go` was removed by `6f49bbf` ("fix(ui): add screen lifecycle cleanup and EventBus unsubscribe on Navigate"). Both regressing commits were merged **after** the v1 verify-report was written.

---

## 2. Area-by-Area Analysis

### 2.1 Navigate (main.go)

**Files retrieved:** `cmd/golemui/main.go` (lines 1-180) — full file. `cmd/golemui/main_test.go` (lines 1-600) — relevant Navigate tests.

**Current state of `ui.Navigate` closure** (`main.go:109-138`):

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
        // Tear down previous screen before loading the new one
        cleanupMu.Lock()
        if prevCleanup != nil {
            prevCleanup()
            prevCleanup = nil
        }
        cleanupMu.Unlock()

        node, err := ui.LoadScreen(ctx, dbPool.CorePool, vID, cfg.LayoutQuery)
        if err != nil {
            log.Printf("[UI/Navigation] Error loading screen %q: %v", vID, err)
            return
        }
        newUI, cleanup, err := ui.Compose(node, vID)
        if err != nil {
            log.Printf("[UI/Navigation] Error composing screen %q: %v", vID, err)
            return
        }

        cleanupMu.Lock()
        prevCleanup = cleanup
        cleanupMu.Unlock()

        mainContainer.Objects = []fyne.CanvasObject{newUI}   // line 134
        mainContainer.Refresh()                              // line 135
        navTree.SelectByVistaID(vID)                         // line 136
    }()
}
```

**v1 status of `ui.Navigate`:** The async `go func()` wrap (T-2.1) is preserved. **T-2.2 (the `fyne.Do` wrap around the UI swap) is gone.** The three UI-mutating statements on lines 134-136 execute directly on the goroutine. The v1 apply-report at `apply-report.md:13` claimed "Wrapped UI mutation block ... in `fyne.Do(func() { ... })`" — that wrap was removed by commit `6f49bbf` (see git history).

**Threading analysis:**

| Step | Thread | Notes |
| --- | --- | --- |
| Button tap → `Navigate(vID)` | UI | Fyne dispatches the button callback. |
| `go func() { ... }()` start | new background goroutine | Async kickoff is preserved. |
| `LoadScreen` (`main.go:123`) | background goroutine | Synchronous DB query on the goroutine — still blocks the goroutine, not the UI. |
| `Compose` (`main.go:128`) | background goroutine | Recursively builds widget tree, spawns its own data_grid goroutines. |
| `mainContainer.Objects = ...` (line 134) | **background goroutine** | **UNSAFE** — direct widget mutation. |
| `mainContainer.Refresh()` (line 135) | **background goroutine** | **UNSAFE** — direct widget refresh. |
| `navTree.SelectByVistaID(vID)` (line 136) | **background goroutine** | **UNSAFE** — see §2.2. |

**Existing tests** in `cmd/golemui/main_test.go`:
* `TestNavigate_NonBlocking` (lines 450-490) — replaces `ui.Navigate` with a fake closure; only asserts the outer function returns before a blocked channel signals. **Does not test the real `RunBootstrap` closure** and therefore does not detect the missing `fyne.Do` wrap.
* `TestNavigate_DispatchesUISwapViaFyneDo` (lines 494-538) — runs the real `RunBootstrap`, calls `ui.Navigate("home")`, then polls the `*container.Split` for `Leading != nil && Trailing != nil`. **The assertion is too weak** — it passes regardless of whether the `fyne.Do` wrap is present, because the split itself is constructed during `RunBootstrap` (before the goroutine) and is never destroyed by the goroutine. The test name and docstring reference `fyne.Do`, but the assertion is structural, not behavioral.
* `TestNavigate_LogsErrorWithoutCrash` (lines 543-596) — overrides `ui.Navigate` to a fake closure; **does not exercise the real production path**.

**Required changes for v2:**
1. Re-apply the `fyne.Do(func() { ... })` wrap around the three UI-mutating statements at `main.go:134-136`. The v1 design at `openspec/changes/fyne-thread-safety/design.md` and the spec at `016-fyne-thread-safety-concurrency.md` "TAREA 1" both call this out explicitly.
2. Strengthen `TestNavigate_DispatchesUISwapViaFyneDo` to actually assert that the goroutine's UI mutations are dispatched via `fyne.Do` (e.g. by replacing `ui.Navigate` with a fake that calls a hook function inside its `fyne.Do` block and asserting the hook fires).

### 2.2 Sidebar (sidebar_widget.go)

**Files retrieved:** `pkg/ui/sidebar_widget.go` (lines 1-156) — full file. `pkg/ui/sidebar_widget_test.go` (lines 1-360) — all `SelectByVistaID` and re-entrancy tests.

**Current state of `SelectByVistaID`** (`sidebar_widget.go:33-58`):

```go
func (nt *NavTree) SelectByVistaID(vistaID string) {
    if vistaID == "" { return }
    nodeID, ok := nt.vistaToNode[vistaID]
    if !ok { return }

    ancestors := []string{}
    for cur := nodeID; cur != "" {
        pid := nt.parentOf[cur]
        if pid != "" { ancestors = append(ancestors, pid) }
        cur = pid
    }
    for i := len(ancestors) - 1; i >= 0; i-- {
        nt.tree.OpenBranch(widget.TreeNodeID(ancestors[i]))   // line 52
    }

    nt.navigating.Store(true)
    defer func() { nt.navigating.Store(false) }()
    nt.tree.Select(widget.TreeNodeID(nodeID))                  // line 57
}
```

**v1 status:** v1 did not touch `sidebar_widget.go`. The function was originally called from the UI thread (the v0 `ui.Navigate` closure in `main.go:115` ran on the UI thread). After v1, `ui.Navigate` was made async (`main.go:122`), and now `navTree.SelectByVistaID(vID)` (line 136) runs on a background goroutine. **The v1 design acknowledged this:** `openspec/changes/fyne-thread-safety/design.md` proposed a `fyne.Do` wrap inside `SelectByVistaID` itself. That change was never made.

**Spec area (TAREA 2 in `016-fyne-thread-safety-concurrency.md`):** "En `SelectByVistaID` de `sidebar_widget.go`, asegurar que las operaciones de mutación del widget `Tree` (`nt.tree.OpenBranch` y `nt.tree.Select`) se realicen de manera segura en el hilo de la UI."

**Threading analysis:**

| Call site | Thread | Notes |
| --- | --- | --- |
| `tree.OnSelected` → `Navigate(item.VistaID)` (`sidebar_widget.go:147`) | UI (Fyne dispatch) | `Navigate` then spawns goroutine. |
| `main.go:136` `navTree.SelectByVistaID(vID)` from inside `ui.Navigate` goroutine | **background goroutine** | **UNSAFE** — both `OpenBranch` and `tree.Select` mutate the tree. |
| `sidebar_widget_test.go:299, 352` (test-only) | test goroutine / test goroutine | Tests pass a `vistaID` to `SelectByVistaID`; tests use Fyne's `test.App()` so the tree calls may "work" without `fyne.Do`, but the production goroutine call is the actual concern. |

**Other tree mutations in `sidebar_widget.go`:**
* `label.SetText(item.Titulo)` at `sidebar_widget.go:115` is inside the `tree.UpdateNode` callback (line 113). Fyne dispatches `UpdateNode` on the UI thread, so it is safe as-is. No v2 change needed.

**Required changes for v2:**
1. Wrap both `nt.tree.OpenBranch` (in the loop) and `nt.tree.Select` calls inside a single `fyne.Do(func() { ... })` block. The block must be contiguous (i.e. the ancestor-opening loop and the final select must be in the same `fyne.Do` callback) to preserve the ordering guarantee that the spec implies ("open all ancestors, then select").
2. The `navigating.Store(true)` re-entrancy guard at `sidebar_widget.go:55-56` **must remain outside** the `fyne.Do` block. Otherwise: the `fyne.Do` callback runs later on the UI thread, by which time the guard is already false, defeating the re-entrancy protection. The guard must be set **before** `fyne.Do` is called and reset after. A clean approach is to use `fyne.DoAndWait` (synchronous dispatch, preserves the v0 ordering exactly) or to set the guard true, dispatch via `fyne.Do`, and accept the brief window where the guard is true after dispatch. For safety against re-entrant goroutine calls from outside `SelectByVistaID`, the `fyne.DoAndWait` option is preferable.

**Existing tests** in `pkg/ui/sidebar_widget_test.go`:
* `TestSelectByVistaID_ValidSelectsNode` (line 285), `TestSelectByVistaID_EmptyIsNoOp` (line 309), `TestSelectByVistaID_UnknownIsNoOp` (line 322) — call `SelectByVistaID` from the test goroutine. None assert that the function is safe to call from a background goroutine.
* `TestReentrancyGuardPreventsLoop` (line 339) — uses `navigating` guard. **Will break** if the guard logic is changed; v2 implementation must preserve the guard semantics.

### 2.3 DataGrid (compositor.go)

**Files retrieved:** `pkg/ui/compositor.go` (lines 1-573) — full file. `pkg/ui/compositor_test.go` (lines 1-1786) — surveyed.

**Current state of the model and the three mutating functions** (`compositor.go`):

The `dataGridModel` struct (`compositor.go:20-34`) has been augmented since v1 with a new `refreshMu sync.Mutex` field:

```go
type dataGridModel struct {
    mu            sync.RWMutex
    refreshMu     sync.Mutex // serializes all table.Refresh() calls to prevent concurrent widget mutations  (line 26)
    headers       []string
    ...
    wg            sync.WaitGroup
}
```

**Every `table.Refresh()` and `table.SetColumnWidth()` call site:**

| # | Line | Function | Goroutine | Mechanism | Safe? |
| --- | --- | --- | --- | --- | --- |
| 1 | `compositor.go:371` | `loadMasterBuffer` | `go func()` (line 343) | `refreshMu.Lock`/`Unlock` (lines 368, 374) | **UNSAFE** — mutex serializes competing refreshes but does **not** dispatch to the UI thread. |
| 2 | `compositor.go:373` | `loadMasterBuffer` | same goroutine | same | **UNSAFE** — same reason. |
| 3 | `compositor.go:399` | `filterMasterRows` (empty-snap early return) | event-bus subscriber goroutine → handler (line 217) → `filterMasterRows` (line 224) | `refreshMu.Lock`/`Unlock` (lines 398, 400) | **UNSAFE**. |
| 4 | `compositor.go:437` | `filterMasterRows` (normal exit) | same | `refreshMu.Lock`/`Unlock` (lines 436, 438) | **UNSAFE**. |
| 5 | `compositor.go:519` | `fetchGridDataAsync` | `go func()` (line 488) | `refreshMu.Lock`/`Unlock` (lines 516, 522) | **UNSAFE**. |
| 6 | `compositor.go:521` | `fetchGridDataAsync` | same goroutine | same | **UNSAFE**. |

**v1 status:** the original `fyne.Do` wraps at `compositor.go:371-376`, `:400-402`, `:438-440`, and `:539-544` (per `openspec/changes/fyne-thread-safety/apply-report.md:7-15` and the diff in commit `17575ee`) were removed by `abc18d6` (the data-access refactor) and replaced with `refreshMu.Lock/Unlock`. **The mutex is not equivalent to `fyne.Do`.** The Fyne threading model requires widget mutations on the UI thread; serializing them on a non-UI mutex does not change the thread they execute on.

**Why `refreshMu` was probably added:** the data-access refactor split the `Refresh` and `SetColumnWidth` calls into a `for` loop and a separate `Refresh` call. Without serialization, two goroutines (e.g. a `loadMasterBuffer` completion racing with a `filterMasterRows` triggered by a submit event) could call `table.Refresh()` concurrently, which Fyne's `*tableRenderer` would treat as overlapping refreshes (the same kind of race that v1 was originally trying to fix). The `refreshMu` prevents the *concurrent* refresh race but does not put the calls on the UI thread. It is a partial fix for the symptom (concurrent refreshes) but not the root cause (widget mutations on the wrong thread).

**Spec area (TAREA 3 in `016-fyne-thread-safety-concurrency.md`):** "En las funciones `loadMasterBuffer` y `fetchGridDataAsync` de `compositor.go`, envolver las llamadas a `table.SetColumnWidth` y `table.Refresh()` dentro de un bloque `fyne.Do(func() { ... })` al completarse la carga de datos en segundo plano. En `filterMasterRows` de `compositor.go`, envolver la llamada a `table.Refresh()` dentro de un bloque `fyne.Do(func() { ... })`."

**`composeWithState` switch coverage:** the v1 spec scoped to `data_grid` only. The v2 spec says: "ALL cases in the `Compose` switch (not just `data_grid` — check `button`, `label`, and other reactive bindings)". Currently, the non-`data_grid` cases are:
* `case "container"`: no goroutines, no UI mutations beyond child composition (which happens on the UI thread during the initial `Compose` call). **No change needed.**
* `case "label"`: `widget.NewLabel(node.Label)` at `compositor.go:142` — synchronous, UI thread. **No change needed for the current label; future reactive-label-binding per `docs/specify/017-reactive-label-binding.md` will need `fyne.Do` wraps** (see §2.4).
* `case "text_input"`: `entry.SetText(node.DefaultValue)` (line 128) and `OnChanged` callback (line 132) — both UI thread (initial composition and Fyne text-input callback). **No change needed.**
* `case "text_area"`: same as text_input. **No change needed.**
* `case "button"`: `widget.NewButton(...)` with a click callback. The click callback runs on the UI thread. The `LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())` call at `compositor.go:151` is safe at the call site (publish itself is thread-safe; the resulting subscriber goroutine is the concern, addressed in §2.4). **No change needed for the current button; future reactive-button-enable per `docs/specify/018-action-button-state-navigation.md` will need `fyne.Do` wraps** (see §2.4).
* `default` (unrecognized): `widget.NewLabel(...)` fallback (line 296) — synchronous, UI thread. **No change needed.**

**Other UI-mutating calls in `compositor.go`:**
* The `table.Length`, `CreateCell`, `UpdateHeader` callbacks (lines 165, 174, 197, 204) are all invoked by Fyne from the UI thread when `Refresh` runs. They are not called directly by GolemUI; they are stored on the table widget. **They will only run on the UI thread once `fyne.Do` is wrapping the `table.Refresh` calls.** Currently, with `table.Refresh` called from a goroutine, Fyne may invoke these callbacks from the wrong thread (per Fyne's contract).
* `table.OnSelected` (line 263) — invoked by Fyne from the UI thread on user click. Its body only reads `model.rows/headers` under `model.mu.RLock` and calls `LocalEventBus.Publish("publish_selection", rowMap)` which is safe at the call site (Publish itself is thread-safe; the goroutine spawned by Publish for the handler is the concern, addressed in §2.4). **No change needed.**
* `entry.SetText` at lines 128 and 139 are called once at composition time, on the UI thread. **No change needed.**

**Required changes for v2:**
1. Replace the `refreshMu.Lock/Unlock` around the three call-site pairs in `loadMasterBuffer` (lines 368-374), `filterMasterRows` empty-snap path (lines 398-400), `filterMasterRows` filtered path (lines 436-438), and `fetchGridDataAsync` (lines 516-522) with `fyne.Do(func() { ... })`. Keep the `model.mu.Unlock()` **before** the `fyne.Do` block at each site (REQ-LOCK-01 from the v1 spec). See §3.3 for the deadlock ordering argument.
2. Decide whether to keep `refreshMu` or remove it. The `fyne.Do` dispatch from a non-test app serializes calls on the Fyne UI thread, so the concurrent-refresh race is also resolved by `fyne.Do` alone. The cleanest design is to **remove `refreshMu`** entirely once `fyne.Do` is in place. (If retained as belt-and-suspenders, it adds no harm but does not address the spec's thread-safety requirement.) Recommendation: remove.
3. Add TDD tests that explicitly assert the `fyne.Do` dispatch (similar to the v1 spec's TDD-04/05/06).

**Existing tests** in `pkg/ui/compositor_test.go`:
* `TestCompose_DataGrid_Success` (line 200), `TestCompose_DataGrid_ReactiveFiltering` (line 325), `TestCompose_DataGrid_ServerMode_SubmitChannelQuery` (line 629), `TestCompose_DataGrid_ClientMode_EagerLoadAndFilter` (line 735), `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` (line 1163), `TestCompose_DataGrid_RowSelection_OutOfBounds_NoPublish` (line 1249), `TestCompose_DataGrid_RowSelection_NilEventBus_NoPanic` (line 1311), `TestCompose_DataGrid_ColumnWidthFromCWR` (line 1593), `TestCompose_DataGrid_ColumnWidthFallback` (line 1649), `TestCompose_DataGrid_DynamicQueryFromState` (line 1697), `TestCompose_ButtonNavigation` (line 1363) — all use `time.Sleep(500ms)` or `time.Sleep(1s)` polling to wait for async work. None assert that the work happens on the Fyne UI thread. Under `go test -race`, the v1 verify-report noted: "all races in Fyne's internal test driver infrastructure (`expiringCache.setAlive`, font metrics cache), not in GolemUI code." With `fyne.Do` re-applied, the same Fyne test driver races are expected (the test driver's `DoFromGoroutine` is non-serialized) but the GolemUI-side unsafety that v1 was originally meant to address is no longer masked.

### 2.4 EventBus Reactive Bindings (compositor.go + eventbus.go)

**Files retrieved:** `pkg/eventbus/eventbus.go` (lines 1-82) — full file. `pkg/ui/compositor.go` lines 211-257 (subscriber handler) — re-read for context.

**Current state of `InMemEventBus.Publish`** (`eventbus.go:65-81`):

```go
func (b *InMemEventBus) Publish(channel string, payload interface{}) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    subs, exists := b.subscribers[channel]
    if !exists { return }

    event := Event{Channel: channel, Payload: payload}

    for _, handler := range subs {
        h := handler
        go h(event)        // every subscriber runs in a fresh goroutine
    }
}
```

**Key fact:** every `Publish` spawns one goroutine per subscriber, and every subscriber handler runs in a goroutine. The GolemUI subscribers today are:
1. The data_grid subscriber at `compositor.go:216-257` — runs in a goroutine, calls `filterMasterRows` or `fetchGridDataAsync` (both of which then call `table.Refresh` from a non-UI goroutine; see §2.3).
2. (No other subscribers exist in production code at the moment; e.g. `row.OnSelected` publishes to `"publish_selection"` but no current code path consumes that channel — see §2.4 follow-up.)

**Forward-looking reactive bindings (per `017` and `018`):**
* `docs/specify/017-reactive-label-binding.md` adds a reactive label: a `widget.Label` that subscribes to a channel (configured via `DataSource` like `"event:..."` or `"publish_selection"`), receives a `map[string]any` payload, resolves `{token}` placeholders in `node.Label` via Dot-Notation (`resolvePath`), and updates the visible text via `label.SetText(...)`. **The spec explicitly mandates `fyne.Do(func() { ... })` around the `label.SetText` call** (§"Actualización Segura en UI" in 017). This is the canonical "label reactive binding" that v2 is meant to cover.
* `docs/specify/018-action-button-state-navigation.md` adds reactive button enable/disable: a `widget.Button` that starts disabled, subscribes to a selection channel (e.g. `"publish_selection"`), and toggles `button.Enable()` / `button.Disable()` on event. **The spec explicitly mandates `fyne.Do(func() { ... })`** around both the `Enable` and `Disable` calls (§1 of 018).

These reactive bindings have **not been implemented yet** (they live in `docs/specify/` but not in `openspec/changes/`). The v2 spec is preparing the safety net: it wants `fyne.Do` to be the established pattern **before** the reactive bindings land, so the new code in `017` and `018` has a clear precedent and template to follow.

**Subscribers to watch (forward-looking):**
* `LocalEventBus.Subscribe("publish_selection", handler)` for the reactive label (017) and the reactive button (018). The current `data_grid.OnSelected` already publishes to this channel at `compositor.go:278`. Once 017/018 land, that channel will have subscribers running in goroutines; the v2 `fyne.Do` pattern must be in place.

**Required changes for v2:**
1. **Re-apply the v1 `fyne.Do` wraps in `compositor.go` (§2.3)** so that the data_grid subscriber handler's `table.Refresh` calls are safe.
2. **Add a documented pattern** in the package comment of `compositor.go` (or a dedicated `docs/sdd/fyne-thread-safety-v2/pattern.md`) that any future `LocalEventBus.Subscribe(...)` handler that mutates a widget must wrap the mutation in `fyne.Do`. This is preparation for 017/018.
3. **Strengthen the existing tests** in `pkg/ui/compositor_test.go` so they fail loudly if a future `LocalEventBus.Subscribe` callback forgets the `fyne.Do` wrap. (Concrete proposal: a test helper `assertFyneDoWithin(t, fn, timeout)` that takes a callback, runs it, and fails if Fyne's `DoFromGoroutine` was not invoked. The Fyne test driver limitation noted in the v1 verify-report is acknowledged, but a counter on `fyne.Do` calls can be a coarse check.)
4. **Optionally add an opt-in assertion** in development builds that, when a widget mutation is detected from a non-UI goroutine, panics. The v1 verify-report flagged this as a Fyne-driver limitation; in the v2 design, this is again a "best effort" check.

**EventBus package itself (`eventbus.go`):** no v2 change is needed in the bus. The `go h(event)` pattern at line 79 is intentional and is what gives GolemUI reactive UI its non-blocking property. The contract is: "the bus does not run handler logic on the UI thread; subscribers are responsible for dispatching their own widget mutations onto the UI thread." That contract is what `fyne.Do` enforces at the GolemUI side. The spec does not propose changing the EventBus goroutine model — and explicitly excludes this in "Fuera de Alcance" / "Restringe el uso de `fyne.Do` únicamente a operaciones que involucren el estado gráfico o estructural de la interfaz de usuario".

---

## 3. Gap Analysis — Unwrapped UI Mutations

Complete inventory of UI-mutating calls from background goroutines in the current codebase. "UI-mutating" = any call on a Fyne widget that alters its visual or structural state. "Background goroutine" = not the Fyne UI thread at the time of the call.

| # | File:Line | Call | Calling context | Thread of caller | Currently wrapped? | Spec area |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `main.go:134` | `mainContainer.Objects = ...` | `ui.Navigate` closure body inside `go func()` | background goroutine | ❌ NO (no `fyne.Do`) | Navigate (TAREA 1) |
| 2 | `main.go:135` | `mainContainer.Refresh()` | same | background goroutine | ❌ NO | Navigate (TAREA 1) |
| 3 | `main.go:136` → `sidebar_widget.go:52` (×N) | `nt.tree.OpenBranch(...)` (in a loop) | `SelectByVistaID` called from background goroutine | background goroutine | ❌ NO | Sidebar (TAREA 2) |
| 4 | `main.go:136` → `sidebar_widget.go:57` | `nt.tree.Select(...)` | `SelectByVistaID` called from background goroutine | background goroutine | ❌ NO | Sidebar (TAREA 2) |
| 5 | `compositor.go:371` | `table.SetColumnWidth(i, w)` (in a loop) | `loadMasterBuffer` goroutine | background goroutine | ❌ NO (only `refreshMu.Lock/Unlock`) | DataGrid (TAREA 3) |
| 6 | `compositor.go:373` | `table.Refresh()` | `loadMasterBuffer` goroutine | background goroutine | ❌ NO | DataGrid (TAREA 3) |
| 7 | `compositor.go:399` | `table.Refresh()` | `filterMasterRows` empty-snap early return, called from event-bus subscriber goroutine | background goroutine | ❌ NO | DataGrid (TAREA 3) + EventBus (TAREA 4) |
| 8 | `compositor.go:437` | `table.Refresh()` | `filterMasterRows` filtered exit, called from event-bus subscriber goroutine | background goroutine | ❌ NO | DataGrid (TAREA 3) + EventBus (TAREA 4) |
| 9 | `compositor.go:519` | `table.SetColumnWidth(i, w)` (in a loop) | `fetchGridDataAsync` goroutine | background goroutine | ❌ NO | DataGrid (TAREA 3) + EventBus (TAREA 4, when called from event-bus subscriber) |
| 10 | `compositor.go:521` | `table.Refresh()` | `fetchGridDataAsync` goroutine | background goroutine | ❌ NO | DataGrid (TAREA 3) + EventBus (TAREA 4, when called from event-bus subscriber) |

**Forward-looking entries (not present in code yet, expected after 017/018 land):**

| # | Source spec | Call | Calling context | Wrapped? |
| --- | --- | --- | --- | --- |
| F1 | 017 §5 | `label.SetText(resolvedText)` in a reactive label subscriber handler | event-bus subscriber goroutine | **WILL BE WRAPPED** per 017 spec (5th bullet) — must be in `fyne.Do`. v2 will be the test of this pattern. |
| F2 | 018 §1 | `button.Enable()` / `button.Disable()` in a reactive button subscriber handler | event-bus subscriber goroutine | **WILL BE WRAPPED** per 018 spec. |
| F3 | 018 §3 | `mainContainer.Objects` / `mainContainer.Refresh()` in the `ui.Navigate` query-string handler | `ui.Navigate` goroutine (extended to parse query string) | **WILL BE WRAPPED** per 018 spec. |

**Total: 10 unwrapped UI mutations in the current code; 3 forward-looking entries in the 017/018 specs that will need the same pattern.**

---

## 4. Dependency Map

Files to be edited for v2 implementation, in dependency order:

```
1. pkg/ui/compositor.go  ──[wrap table.Refresh/SetColumnWidth in fyne.Do at 6 sites;
   │                          remove refreshMu mutex (optional);
   │                          add package comment documenting the fyne.Do pattern]
   │
2. pkg/ui/sidebar_widget.go  ──[wrap nt.tree.OpenBranch loop + nt.tree.Select in fyne.Do;
   │                            preserve navigating re-entrancy guard semantics]
   │
3. cmd/golemui/main.go  ──[wrap mainContainer.Objects/Refresh + navTree.SelectByVistaID
                                in fyne.Do inside the ui.Navigate goroutine]
   │
4. pkg/eventbus/eventbus.go  ──[NO CHANGES; goroutine-per-handler pattern is intentional
                                    and not modified by this spec]
   │
5. pkg/ui/compositor_test.go  ──[add TDD-04/05/06 equivalents that assert fyne.Do dispatch]
   │
6. cmd/golemui/main_test.go  ──[strengthen TestNavigate_DispatchesUISwapViaFyneDo to
                                  actually assert fyne.Do was called]
   │
7. pkg/ui/sidebar_widget_test.go  ──[add a test that calls SelectByVistaID from a
                                       background goroutine and asserts no panic /
                                       no Fyne-thread error]
```

**Cross-cutting concerns:**
* **Deadlock ordering (REQ-LOCK-01 from v1 spec):** at every wrap site, `model.mu.Unlock()` **must** precede the `fyne.Do(...)` block. The `table.Length`, `CreateCell`, and `UpdateHeader` callbacks (compositor.go:165, 174, 204) acquire `model.mu.RLock()` when Fyne invokes them from the UI thread; if the model is locked at the time of the `fyne.Do` dispatch, the UI thread will deadlock. The current `refreshMu`-only design dodges this because the callbacks are Fyne-dispatched synchronously on the calling (background) goroutine — but they will deadlock the moment `fyne.Do` serializes them onto the UI thread while the model is locked. The v1 spec called this out explicitly; v2 must preserve the unlock-then-dispatch ordering.
* **Fyne version:** `go.mod` already has `fyne.io/fyne/v2 v2.7.4` (line 4). `fyne.Do` is available (introduced in v2.6.0). No `go.mod` changes are needed.
* **Test environment caveat (from v1 verify-report):** the Fyne test driver's `DoFromGoroutine` (`test/driver.go:56` in Fyne v2.7.4) runs `fn()` inline on the calling goroutine, not on a serialized UI thread. This means in tests, `fyne.Do(func() { ... })` is *synchronous* but *not serialized across goroutines* — concurrent `fyne.Do` calls in tests will still race on shared Fyne internals (`expiringCache.setAlive`, font metrics). The v2 design must accept this and not promise false race-detector cleanliness in the test suite.

---

## 5. Risks

1. **Re-application risk: `fyne.Do` could mask a different regression.** The v1 PR was verified by the v1 verify-report, but two subsequent refactors silently removed the wraps. If v2 re-applies the wraps without understanding *why* the refactors removed them, the refactors' authors may have legitimate reasons that v2 needs to address. Specifically, `abc18d6` added the `wg sync.WaitGroup` and `model.cancel` cancel-and-wait pattern — the data_access refactor was trying to manage goroutine lifecycle, not concurrency. v2's `fyne.Do` should not break that lifecycle: the `fyne.Do` callback is fire-and-forget, so the data_grid goroutine can exit before the dispatch completes. **Confirm: is this acceptable?** If not, use `fyne.DoAndWait` in some sites (e.g. the navigate UI swap, to ensure the new screen is visible before the goroutine exits).

2. **Re-entrancy guard ordering in `SelectByVistaID`.** The current code at `sidebar_widget.go:55-56` does `nt.navigating.Store(true); defer ... .Store(false)`. If v2 wraps the tree calls in `fyne.Do`, the store-false via `defer` happens *after* the `fyne.Do` call returns, not after the callback executes. This window — between `fyne.Do` returning and the callback actually running on the UI thread — leaves `navigating` set to `true`, which means: any `OnSelected` triggered by another Fyne call in this window will be suppressed. This is *probably* fine (the only thing that could call `OnSelected` is the very `tree.Select` we are dispatching, and we don't want it to re-enter anyway), but it deserves a test. Recommendation: use `fyne.DoAndWait` to keep the v0 timing semantics exactly.

3. **Test driver limitation carries forward.** As the v1 verify-report noted, `go test -race` will continue to surface Fyne-internal races that are not GolemUI bugs. v2 must not promise "race-clean" tests; it can only promise "no GolemUI code path calls a widget method from a non-UI goroutine without a `fyne.Do` wrapper."

4. **Forward-looking reactive bindings may add more sites.** When `017` and `018` land, they will introduce `label.SetText`, `button.Enable/Disable`, and extended `ui.Navigate` query-string handling. v2's deliverable should include a **documented pattern** (in `compositor.go`'s package comment or a separate `docs/`) so that 017/018 implementers can mechanically apply the same `fyne.Do` template.

5. **`ui.Navigate` query-string extension (per 018) may overlap v2.** Spec 018 says "Envolver el refresco de pantalla final de `mainContainer` y de `navTree` en `fyne.Do(func() { ... })`." If 018 lands before v2, the Navigate wrap is already done — v2's TAREA 1 becomes a no-op. If v2 lands first, 018 must not regress. **Coordination needed: which change goes first?** The v2 change is a prerequisite for 018 in the sense that 018's Navigate wrap is one of v2's six sites; landing v2 first eliminates the need for 018 to touch the wrap, and 018 can then focus on its query-string parsing. Recommendation: v2 first, then 018.

6. **No `fyne.Do` is currently in production code (zero matches in `*.go` outside tests).** The first introduction will look like a large diff, but it is largely restoring the v1 work that was removed. The proposal phase should be explicit about the "this is re-applying v1" framing to avoid reviewer confusion when comparing against v1's verify-report (which described the same code).

7. **`refreshMu` removal vs. retention.** If v2 removes `refreshMu` (recommended), the data_access refactor's intent is partially undone. If v2 keeps it, the code carries two concurrency mechanisms (`fyne.Do` + `refreshMu`), which is confusing. The proposal must decide. The strongest argument for `fyne.Do`-only is "single mechanism, matches Fyne's contract." The strongest argument for keeping `refreshMu` is "the data_access refactor explicitly added it to fix a regression it observed; we don't know the regression it was guarding against and removing it might re-expose it." Recommend: keep `refreshMu` for v2, add `fyne.Do` alongside, then deprecate `refreshMu` in a follow-up. This minimizes blast radius for v2.

---

## 6. Files Retrieved

1. `cmd/golemui/main.go` (lines 1-180) — full file. Contains `ui.Navigate` closure, `win.SetContent`, bootstrap flow.
2. `pkg/ui/compositor.go` (lines 1-573) — full file. Contains `dataGridModel`, all `Compose` switch cases, `loadMasterBuffer`, `filterMasterRows`, `fetchGridDataAsync`, `resolveWidth`, `extractOrderedArgs`.
3. `pkg/ui/sidebar_widget.go` (lines 1-156) — full file. Contains `NavTree`, `SelectByVistaID`, `BuildNavTree`, `tree.OnSelected`.
4. `pkg/eventbus/eventbus.go` (lines 1-82) — full file. Contains `InMemEventBus.Publish` (line 79: `go h(event)`).
5. `pkg/ui/screen_state.go` (lines 1-60) — full file. Thread-safe state store backing the EventBus payloads.
6. `pkg/ui/datasource.go` (lines 1-100) — full file. `DataSource` and `ColumnWidthResolver` interfaces; explains the data-access refactor that removed the v1 wraps.
7. `pkg/ui/compositor_test.go` (lines 1-1786) — surveyed. Identified 11 `TestCompose_DataGrid_*` tests and `TestCompose_ButtonNavigation`; all use `time.Sleep` polling.
8. `pkg/ui/sidebar_widget_test.go` (lines 1-360) — full file. 4 `SelectByVistaID` tests + re-entrancy guard test + Tree update tests.
9. `cmd/golemui/main_test.go` (lines 1-600) — full file. Includes 3 `TestNavigate_*` tests; identified weak assertions in `TestNavigate_DispatchesUISwapViaFyneDo`.
10. `docs/specify/016-fyne-thread-safety-concurrency.md` — the v2 originating spec.
11. `docs/specify/017-reactive-label-binding.md` — forward-looking reactive label spec.
12. `docs/specify/018-action-button-state-navigation.md` — forward-looking reactive button + query-string navigation spec.
13. `openspec/changes/fyne-thread-safety/explore.md` — v1 explore, for comparison.
14. `openspec/changes/fyne-thread-safety/apply-report.md` — v1 apply report.
15. `openspec/changes/fyne-thread-safety/verify-report.md` — v1 verify report.
16. `openspec/changes/code-audit-remediation/exploration.md` — code audit that identified the same thread-safety issues and recommended `fyne.Do` (Option A) over the mutex approach (Option B).
17. `git log pkg/ui/compositor.go` (locally) — confirmed `17575ee` added `fyne.Do`, `abc18d6` removed it.
18. `git log cmd/golemui/main.go` (locally) — confirmed `17575ee` added `fyne.Do`, `6f49bbf` removed it.
19. `git show 17575ee -- pkg/ui/compositor.go` (locally) — full diff of the v1 wrap.
20. `git show 17575ee -- cmd/golemui/main.go` (locally) — full diff of the v1 wrap.
21. `git show 6f49bbf -- cmd/golemui/main.go` (locally) — full diff of the regression.
22. `git show abc18d6 -- pkg/ui/compositor.go` (locally) — full diff of the regression.

---

## 7. Start Here

Begin with **`pkg/ui/compositor.go:368-374`** (the `loadMasterBuffer` goroutine). This is the most self-contained site: a single `refreshMu.Lock/Unlock` block contains a tight loop of `table.SetColumnWidth` + one `table.Refresh()`. Re-wrapping it in `fyne.Do` requires only:

1. Move the `refreshMu.Unlock()` to after the `fyne.Do` block (or remove the mutex entirely per Risk #7).
2. Insert `fyne.Do(func() {` before the `for` loop and `})` after `table.Refresh()`.
3. Confirm `model.mu.Unlock()` (line 362) still precedes the new `fyne.Do` block — this preserves REQ-LOCK-01.

Once that one site is clean and tested, the other five follow mechanically: same pattern in `filterMasterRows` (twice), `fetchGridDataAsync`, and finally `cmd/golemui/main.go:134-136`. `pkg/ui/sidebar_widget.go:SelectByVistaID` is the only site that needs more thought (re-entrancy guard ordering — see §2.2 and Risk #2).

For the test side, start with **`pkg/ui/compositor_test.go`** and add a test analogous to v1's TDD-04 (loadMasterBuffer wrap) — the v1 spec at `openspec/changes/fyne-thread-safety/spec.md:75-90` provides a template.
