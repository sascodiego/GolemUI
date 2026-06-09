# SDD Proposal — fyne-thread-safety

> Fix Fyne thread-safety violations in GolemUI: wrap all widget mutations in `fyne.Do()` and make `ui.Navigate` non-blocking.

## 1. Problem Statement

GolemUI violates Fyne's threading model in two distinct ways, causing UI freezes and data races:

### 1.1 Unsafe widget mutations from background goroutines

Six direct calls to `table.Refresh()` and `table.SetColumnWidth()` in `pkg/ui/compositor.go` execute on background goroutines without dispatching onto the Fyne UI thread. Per Fyne's contract, all widget mutations must occur on the main goroutine. These violations produce:

- **Data races** confirmed by `go test -race` (`tableRenderer.Refresh` race, documented in the archived verify-report `2026-06-05-screen-state-store`).
- **Undefined behavior** — Fyne's internal renderer state can corrupt silently under concurrent mutation.

| Site | Call | Goroutine | Source |
|------|------|-----------|--------|
| `compositor.go:370-374` | `SetColumnWidth` + `Refresh` | G1 (`loadMasterBuffer` goroutine) | Background DB query |
| `compositor.go:397-398` | `Refresh` | G3 (EventBus handler → `filterMasterRows`) | Search button submit |
| `compositor.go:433-434` | `Refresh` | G3 (EventBus handler → `filterMasterRows`) | Search button submit |
| `compositor.go:533-536` | `SetColumnWidth` + `Refresh` | G2 (`fetchGridDataAsync` goroutine) | Server-mode query |

`fyne.Do` is used **zero times** anywhere in the project today.

### 1.2 Synchronous navigation blocks the UI thread

`ui.Navigate` (defined at `cmd/golemui/main.go:100-116`) runs entirely on the Fyne UI thread because Fyne dispatches button-tap callbacks on the UI thread. Inside `Navigate`, `ui.LoadScreen` performs a synchronous database query, and `ui.Compose` recursively builds the widget tree. Both block the UI for the full duration of the query + composition, causing visible freezes on every navigation event.

The current code is structurally safe (it happens to be on the correct thread), but functionally wrong — the user cannot interact with the application while a screen loads.

## 2. Proposed Solution

Two independent work streams that can be implemented and tested separately.

### 2.1 Work Stream A — Compositor: `fyne.Do` wrapping

Wrap every `table.Refresh()` and `table.SetColumnWidth()` call that occurs inside a goroutine in `fyne.Do(func() { ... })`.

**Four wrap points:**

1. **`loadMasterBuffer` goroutine** (`compositor.go:370-374`): after `model.mu.Unlock()`, wrap the `SetColumnWidth` loop and `table.Refresh()` in a single `fyne.Do` block.

2. **`filterMasterRows` early return** (`compositor.go:397-398`): when `masterRows` is empty but a snapshot filter was provided, the code shows all rows and calls `table.Refresh()`. Wrap in `fyne.Do`.

3. **`filterMasterRows` normal exit** (`compositor.go:433-434`): after filtering rows, wrap `table.Refresh()` in `fyne.Do`.

4. **`fetchGridDataAsync` goroutine** (`compositor.go:533-536`): after `model.mu.Unlock()`, wrap the `SetColumnWidth` loop and `table.Refresh()` in a single `fyne.Do` block.

**Critical ordering constraint:** `model.mu.Unlock()` must execute **before** entering the `fyne.Do` block. When `table.Refresh()` runs on the UI thread inside `fyne.Do`, it triggers the `Length()` and `CreateCell()` callbacks, which acquire `model.mu.RLock()`. Holding the write lock during `fyne.Do` would deadlock (same-goroutine RLock after Lock). The existing code already unlocks before the refresh calls — this ordering must be preserved.

### 2.2 Work Stream B — Navigation: async dispatch

Convert `ui.Navigate` from synchronous to asynchronous:

1. Spawn a goroutine to execute `LoadScreen` + `Compose`.
2. On success, wrap `mainContainer.Objects` mutation, `mainContainer.Refresh()`, and `navTree.SelectByVistaID(vID)` in `fyne.Do(func() { ... })`.
3. On error from either `LoadScreen` or `Compose`, log the error and return from the goroutine. The previous UI remains visible and responsive.

The `ui.Navigate` function signature (`func(vistaID string)`) is unchanged. The caller (button-tap callback) returns immediately, unblocking the Fyne UI thread.

**Note on `win.SetContent`:** The only `win.SetContent` call in the project occurs at bootstrap (`main.go:138`), before `win.ShowAndRun()`. The `Navigate` callback never calls `win.SetContent` — it only mutates `mainContainer.Objects`. The spec's mention of "win.SetContent inside fyne.Do" translates to: the container mutation that replaces the visible screen content must happen inside `fyne.Do`, which our design satisfies.

## 3. Key Decisions and Rationale

| Decision | Rationale |
|----------|-----------|
| Use `fyne.Do`, not `fyne.DoAndWait` | `fyne.Do` is non-blocking — it queues the function onto the UI thread and returns immediately. This is the correct primitive for "fire-and-forget" UI updates from background goroutines. `DoAndWait` would block the calling goroutine until the UI thread executes the callback, adding unnecessary latency with no benefit. |
| Single `fyne.Do` block per site (SetColumnWidth + Refresh together) | Both calls mutate the same widget and should be atomic from the UI thread's perspective. A single block avoids two separate UI-thread dispatches for what is logically one update. |
| `model.mu.Unlock()` before `fyne.Do` | Prevents deadlock: `table.Refresh()` triggers `Length()` / `CreateCell()` callbacks that acquire `model.mu.RLock()`. Holding the write lock during `fyne.Do` would deadlock since the same goroutine would need to re-acquire as reader. This is a previously-identified constraint from the `screen-state-store` change. |
| No change to `EventBus.Publish` goroutine model | The EventBus dispatches every subscriber handler in `go h(event)` (eventbus.go:79). This is the correct architectural choice — it prevents slow handlers from blocking publishers. The fix belongs at the call site (wrapping the eventual `table.Refresh` in `fyne.Do`), not by changing the bus. |
| No change to `ui.Navigate` signature | The spec explicitly requires preserving the current signature. The function remains `func(vistaID string)`; the caller cannot tell whether execution is synchronous or asynchronous, which is the desired decoupling. |
| No loading indicator in this change | A loading spinner or transition indicator is a UX enhancement, not a thread-safety fix. Adding one would require design decisions (placeholder widget vs. overlay, animation timing, cancellation) outside the scope of this debt-reduction change. |

## 4. Non-Goals

- **No `LoadScreen` internal refactoring.** The DB query logic, SQL parameters, and connection pool usage remain untouched.
- **No changes to `EventBus.Publish`.** The goroutine-per-handler model stays as-is.
- **No changes to non-`data_grid` widget cases.** Container, label, text_input, text_area, and button cases do not spawn goroutines and are already thread-safe.
- **No changes to `NodeMeta` struct or its properties.**
- **No loading indicator / transition animation.** This is a thread-safety fix, not a UX polish.
- **No `win.SetContent` changes.** Called once at bootstrap before the event loop starts; no thread-safety issue.
- **No cancellation or debouncing for rapid navigation clicks.** The user can tap multiple navigation buttons quickly; each will spawn a goroutine that completes and swaps the container. Sequential ordering is not guaranteed but the last `fyne.Do` wins — acceptable for a first fix. A navigation guard or cancellation token is a separate enhancement.

## 5. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Deadlock from lock ordering** — `model.mu` write lock held during `fyne.Do` causes deadlock when `Refresh` callbacks try to `RLock` | Medium (known historical issue) | High (application hangs) | Enforce `model.mu.Unlock()` **before** `fyne.Do` at every site. Verify with test that explicitly checks for completion within a timeout. Code review checklist item. |
| **Race between rapid navigation goroutines** — user clicks two nav buttons quickly; two goroutines both complete and call `fyne.Do` to swap the container | Low (depends on user behavior) | Low (last writer wins; UI shows correct screen) | Acceptable for first fix. A navigation guard (mutex or cancellation token) can be added later if users report confusion. |
| **`fyne.Do` semantics in test environment** — `fyne.Do` may not dispatch correctly without a running event loop | Medium | Medium (tests may hang or false-pass) | Fyne's `test.App()` provides a synchronous `fyne.Do` implementation for tests. Verify test behavior with the Fyne test harness. |
| **Existing tests may break** — tests that implicitly relied on synchronous `Navigate` may now see empty/stale state | Low | Medium | Audit all `TestRunBootstrap_*` and `TestCompose_ButtonNavigation` tests. Adjust timing or add synchronization where needed. |
| **`filterMasterRows` has two `table.Refresh()` sites** — the early-return path is easy to miss during implementation | Low | Medium (one site left unsafe) | Enumerate all 6 call sites in tasks and verify each with `grep` after implementation. |

## 6. Success Criteria

Matching the acceptance criteria from the originating spec (`docs/specify/a013-fyne-thread-safety-and-async-screen-loading.md`):

1. **AC-1 — Non-blocking Navigate:** When `ui.Navigate` is invoked, the call returns control to the calling thread immediately without blocking (asynchronous behavior). Validated by a test that asserts the call returns before the underlying `LoadScreen` completes.

2. **AC-2 — UI update dispatched via `fyne.Do`:** The final container mutation (`mainContainer.Objects = …` + `mainContainer.Refresh()` + `navTree.SelectByVistaID`) inside `ui.Navigate` executes within a `fyne.Do()` block. Validated by code inspection and test.

3. **AC-3 — All table mutations wrapped in `fyne.Do`:** Every `table.Refresh()` and `table.SetColumnWidth()` call inside a goroutine in `compositor.go` is encapsulated in `fyne.Do()`. Validated by `grep` audit (zero unwrapped calls from goroutine context) and `go test -race` passing cleanly.

4. **AC-4 — Error handling:** Errors from `LoadScreen` or `Compose` in the background goroutine are logged. The previous UI remains visible and responsive. Validated by test that injects a failing `LoadScreen` and asserts the UI is unchanged.

## 7. Files Affected

| File | Change type | Description |
|------|------------|-------------|
| `pkg/ui/compositor.go` | Modify | Wrap 6 unsafe `table.Refresh()` / `SetColumnWidth()` calls in `fyne.Do()` at 4 sites |
| `cmd/golemui/main.go` | Modify | Convert `ui.Navigate` to async: goroutine for LoadScreen+Compose, `fyne.Do` for UI swap |
| `cmd/golemui/main_test.go` | Add tests | Non-blocking Navigate test, error handling test |
| `pkg/ui/compositor_test.go` | Add tests | Thread-dispatch assertions for data_grid table mutations |
