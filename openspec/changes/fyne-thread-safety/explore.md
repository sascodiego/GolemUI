# SDD Explore — fyne-thread-safety

> Mapping the current state of Fyne thread-safety in GolemUI's UI code. Scope: `cmd/golemui/main.go` and `pkg/ui/compositor.go` (plus supporting reads in `pkg/eventbus/eventbus.go` and the relevant tests).

## 1. Summary of findings (TL;DR)

GolemUI's UI is **not thread-safe today**:

- `ui.Navigate` (in `main.go`) runs **synchronously on the Fyne UI thread** when invoked by a button tap. Inside, it calls `ui.LoadScreen` and `ui.Compose` on the UI thread, then directly mutates `mainContainer.Objects` and calls `mainContainer.Refresh()` and `navTree.SelectByVistaID(...)`. That part is *currently* safe, but blocks the UI on every navigation while the DB query runs.
- The bigger problem is in `compositor.go`: every `widget.Table` mutation that happens **after** a background DB query completes is called from a non-UI goroutine with **no `fyne.Do` wrapper at all**.
  - 4 unsafe `table.Refresh()` calls
  - 2 unsafe `table.SetColumnWidth()` calls (one in a `for` loop)
  - 2 explicit `go func()` goroutines
  - 1 additional implicit goroutine chain via the in-memory `EventBus.Publish`, which itself dispatches every subscriber handler in a `go h(event)` (eventbus.go:79) — the `data_grid` subscriber handler therefore runs on a background goroutine, and the handler in turn calls `filterMasterRows` and `fetchGridDataAsync` from that goroutine.
- `fyne.Do` / `fyne.DoAndWait` is **not used anywhere** in the project (zero matches in `compositor.go`, `main.go`, or any `.go` file in the repo — only mentioned in archived spec text).
- No existing test asserts that `ui.Navigate` is non-blocking, nor that the table refreshes are dispatched on the Fyne UI thread. All `data_grid` tests poll with `time.Sleep(500ms)` to wait for async work.

Net: the project needs both
1. **Compositor fixes** — wrap every `table.Refresh()` and `table.SetColumnWidth()` in `fyne.Do(...)`.
2. **Navigation fix** — make `ui.Navigate` async: spawn a goroutine for `LoadScreen` + `Compose`, then push the final UI swap onto the UI thread via `fyne.Do`.

---

## 2. `cmd/golemui/main.go` — `ui.Navigate` callback

### Location and complete body

`ui.Navigate` is **assigned** at `main.go:101` (the package-level `var Navigate func(vistaID string)` is declared in `pkg/ui/compositor.go:18`). It is a closure over `ctx`, `cfg.LayoutQuery`, `ui.CorePool`, `mainContainer`, and `navTree`.

Full body (`main.go:100–116`):

```go
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

### How it is invoked

`ui.Navigate` is invoked by Fyne button callbacks produced by the compositor in `case "button":` (compositor.go:131–138). Specifically, the button handler is

```go
if strings.HasPrefix(node.SubmitAction, "navigate:") && Navigate != nil {
    targetVista := strings.TrimPrefix(node.SubmitAction, "navigate:")
    return widget.NewButton(node.Label, func() {
        Navigate(targetVista)
    }), nil
}
```

Therefore `ui.Navigate` **runs on the Fyne UI thread** (Fyne dispatches all widget callbacks on the UI thread). The "navigation" itself is synchronous and blocks the UI while `LoadScreen` and `Compose` execute.

### Threading analysis

| Step | Thread | Notes |
| --- | --- | --- |
| Button tap | UI | Fyne dispatch. |
| `ui.LoadScreen(ctx, ui.CorePool, vID, cfg.LayoutQuery)` (line 105) | UI | Synchronous DB query → blocks UI. |
| `ui.Compose(node, vID)` (line 109) | UI | Recursive widget build, spawns its own goroutines for data_grid. Returns when tree is built, but background work continues. |
| `mainContainer.Objects = []fyne.CanvasObject{newUI}` (line 113) | UI | Safe — direct mutation on UI thread. |
| `mainContainer.Refresh()` (line 114) | UI | Safe — direct call on UI thread. |
| `navTree.SelectByVistaID(vID)` (line 115) | UI | Safe — direct call on UI thread. |

`win.SetContent` is **not** called from `Navigate`. The window content is set once during bootstrap:

```go
// main.go:137-138
mainContainer.Objects = []fyne.CanvasObject{homeUI}
win.SetContent(split)
```

The split layout is a stable container; navigation only swaps the right-hand `mainContainer`'s objects.

### Required changes for this function

Per the spec in `docs/specify/a013-fyne-thread-safety-and-async-screen-loading.md` and the natural reading of the Fyne threading model:

1. `LoadScreen` and `Compose` must move into a `go func()` so the button tap returns immediately.
2. The final UI swap (`mainContainer.Objects = …; mainContainer.Refresh(); navTree.SelectByVistaID(vID)`) must be wrapped in `fyne.Do(...)` so the assignment and the `Refresh()` happen on the Fyne UI thread.
3. Errors inside the goroutine must be logged but not allowed to panic.

(`win.SetContent` itself does not need wrapping inside `Navigate` because it is called only once at bootstrap before `win.ShowAndRun()`. The spec wording is slightly misleading on that point.)

---

## 3. `pkg/ui/compositor.go` — goroutines and unsafe UI mutations

### 3.1 Inventory of goroutines

| # | Source line | Function | Spawned by |
| --- | --- | --- | --- |
| G1 | `compositor.go:321` | `loadMasterBuffer` goroutine | `loadMasterBuffer(...)` called from `composeWithState` "data_grid" branch at `compositor.go:198` |
| G2 | `compositor.go:480` | `fetchGridDataAsync` goroutine | `fetchGridDataAsync(...)` called from (a) `composeWithState` "data_grid" branch at `compositor.go:209` and (b) the event-bus subscriber handler at `compositor.go:255` |
| G3 | `eventbus.go:79` | `go h(event)` for every subscriber | Triggered by every `LocalEventBus.Publish(...)` call — including the one in the data_grid handler itself at `compositor.go:137` (button → "search" / submit channel) and the one in `table.OnSelected` at `compositor.go:280` |

The data_grid **subscriber handler** is defined inline at `compositor.go:216` as the callback passed to `LocalEventBus.Subscribe(state.SubmitChannel(), ...)`. Because `EventBus.Publish` (eventbus.go:79) wraps every handler dispatch in `go h(event)`, the entire body of that handler runs in a goroutine.

### 3.2 Every `table.Refresh()` and `table.SetColumnWidth()` call

| Line | Call | Goroutine context | Currently safe? |
| --- | --- | --- | --- |
| `compositor.go:372` | `table.SetColumnWidth(i, 150)` (inside `for i := 0; i < len(headers); i++`) | G1 (loadMasterBuffer goroutine) | **UNSAFE** |
| `compositor.go:374` | `table.Refresh()` | G1 (loadMasterBuffer goroutine) | **UNSAFE** |
| `compositor.go:398` | `table.Refresh()` (early-return branch when `masterRows` is empty) | G3 → handler goroutine (filterMasterRows called at compositor.go:226) | **UNSAFE** |
| `compositor.go:434` | `table.Refresh()` (end of filterMasterRows) | G3 → handler goroutine (filterMasterRows called at compositor.go:226) | **UNSAFE** |
| `compositor.go:534` | `table.SetColumnWidth(i, 150)` (inside `for i := 0; i < len(headers); i++`) | G2 (fetchGridDataAsync goroutine) | **UNSAFE** |
| `compositor.go:536` | `table.Refresh()` | G2 (fetchGridDataAsync goroutine) | **UNSAFE** |

All six sites are **direct calls on a Fyne widget from a non-UI goroutine with no `fyne.Do` wrapper**. Per Fyne's threading contract, this is undefined behavior and historically triggers the `fyne.io/fyne/v2/widget.(*tableRenderer).Refresh` race flagged in `openspec/changes/archive/2026-06-05-screen-state-store/verify-report.md:49-50`.

### 3.3 `fyne.Do` / `fyne.DoAndWait` usage

**None.** `grep -rn "fyne\.Do\|fyne\.DoAndWait" --include="*.go"` in the entire repository returns zero matches. The previous spec (`asynchronous-data-source-querying/spec.md:11`) and the verification report both mention `fyne.Do`, but the code never actually wraps anything in it.

### 3.4 What `loadMasterBuffer` and `fetchGridDataAsync` actually do

`loadMasterBuffer` (`compositor.go:315-377`):

```go
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
    if BusinessPool == nil { … return }
    log.Printf(...)
    go func() {                       // G1 — line 321
        if err := ctx.Err(); err != nil { return }
        rows, err := BusinessPool.Query(ctx, node.MasterDataSource)
        if err != nil { … return }
        defer rows.Close()
        fds := rows.FieldDescriptions()
        var headers []string
        for _, fd := range fds { headers = append(headers, fd.Name) }
        var dataRows [][]string
        for rows.Next() { … vals, err := rows.Values(); … dataRows = append(...) }
        if err := ctx.Err(); err != nil { return }

        // Lock-protected model write — SAFE
        model.mu.Lock()
        model.masterHeaders = headers
        model.masterRows    = dataRows
        model.headers       = headers
        model.rows          = dataRows
        model.mu.Unlock()

        // ⚠ UNSAFE — both calls on goroutine, no fyne.Do
        for i := 0; i < len(headers); i++ {
            table.SetColumnWidth(i, 150)   // line 372
        }
        table.Refresh()                    // line 374
    }()
}
```

`fetchGridDataAsync` (`compositor.go:475-538`): same shape as `loadMasterBuffer` — it spawns goroutine G2 at line 480, runs the query, writes the model under `model.mu`, then **unconditionally** calls `table.SetColumnWidth` and `table.Refresh()` outside any UI dispatcher.

`filterMasterRows` (`compositor.go:380-435`): called from the event-bus subscriber handler at `compositor.go:226` (client mode). Because the handler itself runs in goroutine G3, `filterMasterRows` runs on G3. It mutates `model.rows` under `model.mu` (safe) and then calls `table.Refresh()` twice — at line 398 (empty-buffer early return) and at line 434 (normal exit). Both are unsafe.

### 3.5 The EventBus subscription model

`pkg/eventbus/eventbus.go:79`:

```go
for _, handler := range subs {
    h := handler
    go h(event)        // <-- every subscriber handler runs in its own goroutine
}
```

`LocalEventBus.Subscribe` is invoked for each `data_grid` at `compositor.go:216`. The subscribed handler therefore runs on a fresh background goroutine for every `Publish` on the submit channel.

When a "search" button is tapped (`compositor.go:131-138`), it calls `LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())`. The submit channel is `screen:submit:<vistaID>` (`pkg/eventbus/eventbus.go:10`). Every active `data_grid` on the same screen receives a goroutine — and that goroutine is what eventually calls `table.Refresh()` / `table.SetColumnWidth()` either directly (server mode → G2) or via `filterMasterRows` (client mode → G3).

### 3.6 Other `case` branches and their threading

The non-`data_grid` cases in `composeWithState` (container, label, text_input, text_area, button) do not spawn goroutines. `table.OnSelected` at `compositor.go:266` runs on the UI thread (Fyne dispatches it) and itself only calls `LocalEventBus.Publish("publish_selection", rowMap)` (compositor.go:280), which is safe at the call site because the actual subscriber work moves onto a new goroutine inside `EventBus.Publish`.

---

## 4. Existing tests

### 4.1 `cmd/golemui/main_test.go`

- **No tests reference `ui.Navigate` directly.** The only test that touches navigation is `TestCompose_ButtonNavigation` in `pkg/ui/compositor_test.go` (compositor_test.go:1368-1399), which only swaps in a fake `ui.Navigate = func(...) { navigatedTo = vistaID }` to assert that the button's tap calls it. It does **not** assert that `Navigate` returns immediately, nor anything about the Fyne thread.
- All `TestRunBootstrap_*` tests call `RunBootstrap(ctx, cfg, false, testApp)` with `runWindow: false`, so the Fyne event loop never runs. They never trigger `ui.Navigate` from a real button.
- There is **no** test that:
  - Verifies `ui.Navigate` is non-blocking.
  - Verifies `win.SetContent` is wrapped in `fyne.Do`.
  - Verifies `mainContainer.Refresh()` inside `Navigate` runs on the UI thread.

### 4.2 `pkg/ui/compositor_test.go`

- 9 tests prefixed `TestCompose_DataGrid_` (lines ~188, 248, 275, 360, 668, 805, 1008, 1175, 1332). They all use a `test.NewApp()` (via `test.NewApp()` in `main_test.go` calling into `RunBootstrap`, or directly via the compositor tests) and poll with `time.Sleep(10ms)` inside a 500ms deadline to wait for async data. None of them asserts on Fyne's threading contract.
- One test, `TestCompose_ButtonNavigation` (compositor_test.go:1368-1399), substitutes a fake `ui.Navigate` and `test.Tap(btn)` to verify the button wires correctly.
- A `TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel` (compositor_test.go:1175) exists but it uses `wg.Wait()` inside a goroutine (line 600, 602, 1236) to wait for the publish — it does **not** validate UI thread safety.
- No test uses the Go race detector's pattern (e.g. `go test -race`) to validate that `table.Refresh()` is not called concurrently. The archived verify-report at `2026-06-05-screen-state-store/verify-report.md:49-50` explicitly notes that `-race` flags `tableRenderer.Refresh` races in the existing data_grid tests, attributing them to "pre-existing Fyne test driver race" — i.e. the bug is real, it just hasn't been fixed.

---

## 5. Required changes (proposal-shaped, not yet a proposal)

### 5.1 Compositor

For every site listed in §3.2, wrap the call in `fyne.Do`:

```go
fyne.Do(func() {
    for i := 0; i < len(headers); i++ {
        table.SetColumnWidth(i, 150)
    }
    table.Refresh()
})
```

Six concrete wrap points:

1. `compositor.go:371-374` (loadMasterBuffer goroutine)
2. `compositor.go:398` (filterMasterRows early return)
3. `compositor.go:434` (filterMasterRows end)
4. `compositor.go:533-536` (fetchGridDataAsync goroutine)

The model writes that immediately precede the UI calls stay under `model.mu` (already correct). Make sure the `model.mu.Unlock()` happens **before** the `fyne.Do(...)` block to avoid the deadlock already documented in `screen-state-store/verify-report.md:74` (`filterMasterRows` previously had this fixed for the non-`fyne.Do` code path; the `fyne.Do` wrapper preserves the same property).

### 5.2 Main / `ui.Navigate`

`main.go:100-116` becomes:

```go
ui.Navigate = func(vID string) {
    log.Printf("[UI/Navigation] Navigating to screen %q", vID)
    go func() {
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
        fyne.Do(func() {
            mainContainer.Objects = []fyne.CanvasObject{newUI}
            mainContainer.Refresh()
            navTree.SelectByVistaID(vID)
        })
    }()
}
```

Notes:

- `win.SetContent(split)` at `main.go:138` runs once during bootstrap **before** `win.ShowAndRun()`, so it does not need to move into `fyne.Do`. The spec text mentioning "win.SetContent inside fyne.Do" is satisfied by the fact that the only `SetContent` call is at bootstrap, and the in-`Navigate` mutation only updates `mainContainer.Objects` — which **does** need `fyne.Do` because the UI is live by then.
- `ctx` and `cfg.LayoutQuery` are captured by the closure. They are immutable for the lifetime of `RunBootstrap`, so the goroutine is safe.

### 5.3 Tests to add (success criteria)

1. `TestNavigate_NonBlocking` — invoke `ui.Navigate("...")` from a mock whose `LoadScreen` blocks on a channel; assert that the call returns before the channel is closed.
2. `TestNavigate_UIUpdateDispatched` — wrap `mainContainer.Refresh` in a hook or count its invocations; assert it is called exactly once on the Fyne UI thread.
3. `TestCompose_DataGrid_TableRefreshOnUIThread` — assert that `fyne.Do` (or the equivalent) is called around `table.Refresh()` and `table.SetColumnWidth()`.
4. `go test ./pkg/ui/... -race -count=1` must pass with no `tableRenderer.Refresh` race.

---

## 6. Files retrieved

1. `cmd/golemui/main.go` (lines 1-180) — bootstrap, `ui.Navigate` definition, `win.SetContent` call.
2. `pkg/ui/compositor.go` (lines 1-538) — `Compose`, `composeWithState`, `loadMasterBuffer`, `filterMasterRows`, `fetchGridDataAsync`, all `data_grid` widget logic, every `go func`, every `table.Refresh` and `table.SetColumnWidth` call.
3. `pkg/eventbus/eventbus.go` (lines 1-82) — `Publish` (line 79: `go h(event)`).
4. `cmd/golemui/main_test.go` (lines 1-440) — confirmed no `Navigate` threading tests, all tests use `runWindow: false`.
5. `pkg/ui/compositor_test.go` — surveyed via grep: 9 `TestCompose_DataGrid_*` tests + `TestCompose_ButtonNavigation`; all use time-based polling, no Fyne-thread assertions.
6. `docs/specify/a013-fyne-thread-safety-and-async-screen-loading.md` — the originating spec (read for context).
7. `openspec/changes/archive/2026-06-05-screen-state-store/verify-report.md` — confirms `fyne.Do` was the *intended* pattern and that the current code triggers a `tableRenderer.Refresh` race under `-race`.

## 7. Key constraints / open questions

- The compositor's `data_grid` branch currently subscribes to the submit channel **per-instance** and stores the unsubscribe func on the model (`compositor.go:241-247`). The unsubscribe is never actually invoked; if we move the UI swap to a different goroutine we must keep that pattern.
- `EventBus.Publish` itself dispatches every subscriber in its own goroutine (eventbus.go:79). Even if we wrap `table.Refresh` in `fyne.Do`, the handler still does model work outside the lock in the goroutine — that work is currently safe because the model fields are written under `model.mu`. Do not regress that.
- `loadMasterBuffer` writes `model.rows` and `model.headers` *while* the table is already showing 0×0 cells. The first `table.Refresh()` after the eager load happens from the goroutine, which is exactly the unsafe pattern.
- The archive spec at `screen-state-store/verify-report.md:74` mentions a previously-fixed deadlock where `model.mu.Unlock()` ran *after* `fyne.Do(table.Refresh())`. Any new wrap must keep the unlock-before-`fyne.Do` ordering, because the `Length()` callback inside `Refresh()` re-acquires the same `RLock` via the table callbacks at `compositor.go:152, 161, 178`.
- `win.SetContent` is only called once in the project (at bootstrap). The spec mentions wrapping it in `fyne.Do` inside `Navigate`, but the current `Navigate` does not call it. We should clarify the spec language vs. the actual code; the safe path is: only mutate `mainContainer` from the UI thread (which `fyne.Do` gives us), and leave `SetContent` alone.

## 8. Start here

Begin with `pkg/ui/compositor.go:321-377` (`loadMasterBuffer` and its goroutine) and `pkg/ui/compositor.go:475-538` (`fetchGridDataAsync` and its goroutine). These are the two clearest, most self-contained `fyne.Do` wrap targets. The `filterMasterRows` (`compositor.go:380-435`) and the event-bus handler (`compositor.go:216-258`) are the harder ones because they also model the publish→goroutine→UI chain. Fix those four sites first, then move to `cmd/golemui/main.go:100-116` to make `ui.Navigate` async.
