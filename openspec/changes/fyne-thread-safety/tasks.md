# SDD Tasks — fyne-thread-safety

> Implementation task breakdown for Fyne thread-safety fix and async screen loading.

## 1. Overview

| Phase | Scope | Files | Est. lines | Requirements |
|-------|-------|-------|-----------|-------------|
| Phase 1 | Compositor `fyne.Do` wrapping | `pkg/ui/compositor.go` | ~12 | REQ-THREAD-01, REQ-THREAD-02, REQ-THREAD-03, REQ-LOCK-01 |
| Phase 2 | Async `ui.Navigate` | `cmd/golemui/main.go` | ~8 | REQ-ASYNC-01, REQ-ASYNC-02, REQ-ASYNC-03 |
| Phase 3 | Tests + race validation | `pkg/ui/compositor_test.go`, `cmd/golemui/main_test.go` | ~120 | All TDD scenarios |

**Total estimated diff:** ~140 lines (production + tests).

---

## 2. Phase 1 — Compositor `fyne.Do` Wrapping

Wrap all 6 unsafe widget mutations (at 4 call sites) in `fyne.Do()`. Critical invariant: `model.mu.Unlock()` MUST complete before `fyne.Do()` is entered (REQ-LOCK-01 / deadlock prevention).

### Tasks

- [ ] T-1.1: Add `"fyne.io/fyne/v2"` import to `pkg/ui/compositor.go`
- [ ] T-1.2: Wrap `loadMasterBuffer` goroutine — `table.SetColumnWidth` loop + `table.Refresh()` (~L371-374) in `fyne.Do(func() { ... })`. Verify `model.mu.Unlock()` at ~L360 precedes the `fyne.Do` block. (REQ-THREAD-01, REQ-LOCK-01)
- [ ] T-1.3: Wrap `filterMasterRows` early-return `table.Refresh()` (~L398) in `fyne.Do(func() { table.Refresh() })`. Verify `model.mu.Unlock()` at ~L397 precedes the `fyne.Do` block. (REQ-THREAD-03, REQ-LOCK-01)
- [ ] T-1.4: Wrap `filterMasterRows` normal-exit `table.Refresh()` (~L434) in `fyne.Do(func() { table.Refresh() })`. Verify `model.mu.Unlock()` at ~L433 precedes the `fyne.Do` block. (REQ-THREAD-03, REQ-LOCK-01)
- [ ] T-1.5: Wrap `fetchGridDataAsync` goroutine — `table.SetColumnWidth` loop + `table.Refresh()` (~L533-536) in `fyne.Do(func() { ... })`. Verify `model.mu.Unlock()` at ~L528 precedes the `fyne.Do` block. (REQ-THREAD-02, REQ-LOCK-01)

### Dependencies

T-1.1 must come first (import). T-1.2 through T-1.5 are independent of each other and can be applied in any order.

### Verification

```bash
go build ./pkg/ui/...
go test ./pkg/ui/... -count=1
go vet ./pkg/ui/...
```

---

## 3. Phase 2 — Async `ui.Navigate`

Move `LoadScreen` + `Compose` into a background goroutine so the button-tap callback returns immediately. Wrap the final UI swap in `fyne.Do()`.

### Tasks

- [ ] T-2.1: Refactor `ui.Navigate` closure in `cmd/golemui/main.go` (~L101-116): wrap `LoadScreen` + `Compose` in `go func() { ... }()`. The outer function returns immediately after spawning the goroutine. (REQ-ASYNC-01)
- [ ] T-2.2: Inside the goroutine, wrap the UI mutation block (`mainContainer.Objects` assignment + `mainContainer.Refresh()` + `navTree.SelectByVistaID(vID)`) in `fyne.Do(func() { ... })`. (REQ-ASYNC-02)
- [ ] T-2.3: Preserve error handling — `log.Printf` on `LoadScreen`/`Compose` error, return from goroutine without calling `fyne.Do`. Previous `mainContainer` content stays visible. (REQ-ASYNC-03)

### Dependencies

Phase 2 is logically independent of Phase 1 but should follow it so the race detector is already clean when Navigate is tested.

### Verification

```bash
go build ./cmd/golemui/...
go test ./cmd/golemui/... -count=1
```

---

## 4. Phase 3 — Tests and Race Validation

Add new tests covering all TDD scenarios from the spec. Existing tests must continue passing without modification.

### Tasks

- [ ] T-3.1: `TestNavigate_NonBlocking` — configure `ui.Navigate` with a mock `LoadScreen` that blocks on a channel; assert `Navigate` returns before the channel is signaled. (TDD-01, REQ-ASYNC-01)
- [ ] T-3.2: `TestNavigate_DispatchesUISwapViaFyneDo` — configure `ui.Navigate` with instant mock `LoadScreen`/`Compose`; assert `mainContainer.Objects` is updated after the goroutine completes. (TDD-02, REQ-ASYNC-02)
- [ ] T-3.3: `TestNavigate_LogsErrorWithoutCrash` — configure `ui.Navigate` with a `LoadScreen` that returns an error; assert previous `mainContainer` content unchanged, error logged, no panic. (TDD-03, REQ-ASYNC-03)
- [ ] T-3.4: `TestCompose_DataGrid_ConcurrentOps_NoDeadlock` — compose a `data_grid`, trigger `loadMasterBuffer` (G1) + `fetchGridDataAsync` (G2) + EventBus filter (G3) concurrently; assert all complete within 5 seconds (no deadlock). (TDD-07, REQ-LOCK-01)
- [ ] T-3.5: Run full test suite with race detector — `go test -race ./... -count=1` — and verify zero `tableRenderer.Refresh` races and zero other widget mutation races. (TDD-08, REQ-THREAD-01/02/03)
- [ ] T-3.6: Grep audit — confirm zero unwrapped `table.Refresh()` or `table.SetColumnWidth()` calls from goroutine context in `compositor.go` and `main.go`.

### Dependencies

- T-3.1, T-3.2, T-3.3 depend on Phase 2 being complete.
- T-3.4 depends on Phase 1 being complete.
- T-3.5 and T-3.6 depend on both Phase 1 and Phase 2 being complete.

### Verification

```bash
go test -race ./... -count=1
grep -n "table\.Refresh\|table\.SetColumnWidth" pkg/ui/compositor.go
# All occurrences must be inside fyne.Do() blocks
```

---

## 5. Implementation Order

```
T-1.1 (import)
  ├── T-1.2 (loadMasterBuffer wrap)
  ├── T-1.3 (filterMasterRows early return wrap)
  ├── T-1.4 (filterMasterRows normal exit wrap)
  └── T-1.5 (fetchGridDataAsync wrap)
       │
       ▼  Phase 1 verification: go test ./pkg/ui/... -count=1
       │
T-2.1 + T-2.2 + T-2.3 (async Navigate — single commit)
       │
       ▼  Phase 2 verification: go test ./cmd/golemui/... -count=1
       │
T-3.1, T-3.2, T-3.3 (Navigate tests)
T-3.4 (concurrent deadlock test)
       │
       ▼
T-3.5 (race detector sweep)
T-3.6 (grep audit)
       │
       ▼  Final verification: go test -race ./... -count=1
```

---

## 6. Estimated Changed Lines per Task

| Task | File | Lines changed |
|------|------|--------------|
| T-1.1 | `compositor.go` | +1 (import) |
| T-1.2 | `compositor.go` | ~+3, ~-2 (wrap block) |
| T-1.3 | `compositor.go` | ~+2, ~-1 |
| T-1.4 | `compositor.go` | ~+2, ~-1 |
| T-1.5 | `compositor.go` | ~+3, ~-2 |
| T-2.1–T-2.3 | `main.go` | ~+6, ~-4 |
| T-3.1 | `main_test.go` | ~+25 |
| T-3.2 | `main_test.go` | ~+25 |
| T-3.3 | `main_test.go` | ~+20 |
| T-3.4 | `compositor_test.go` | ~+35 |
| T-3.5 | (CLI only) | 0 |
| T-3.6 | (CLI only) | 0 |
| **Total** | | **~143 lines** |
