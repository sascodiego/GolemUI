# SDD Tasks — screen-lifecycle-cleanup

> Implementation task breakdown for automatic EventBus unsubscription and resource cleanup on screen navigation in GolemUI.

## 1. Overview

| Phase | Scope | Files | Est. lines | Requirements |
|-------|-------|-------|-----------|-------------|
| Phase 1 | Compose signature change + cleanup plumbing | `pkg/ui/compositor.go` | ~+25, ~-5 | REQ-CLEANUP-01, REQ-CLEANUP-05 |
| Phase 2 | Navigate teardown + bootstrap cleanup | `cmd/golemui/main.go` | ~+10, ~-4 | REQ-CLEANUP-02 |
| Phase 3 | EventBus test helper | `pkg/eventbus/eventbus_test.go` | ~+12 | REQ-CLEANUP-03 |
| Phase 4 | New TDD tests | `pkg/ui/compositor_test.go` | ~+80 | REQ-CLEANUP-01/03/04/05 |
| Phase 5 | Navigate TDD test | `cmd/golemui/main_test.go` | ~+30 | REQ-CLEANUP-02 |
| Phase 6 | Migrate existing Compose callers | `pkg/ui/compositor_test.go`, `cmd/golemui/main_test.go` | ~+22 (mechanical) | REQ-CLEANUP-07 |
| Phase 7 | Final verification | CLI only | 0 | All |

**Total estimated diff:** ~+180 lines added, ~-9 lines removed.

**No changes to `pkg/eventbus/eventbus.go`** (REQ-CLEANUP-06).

---

## 2. Phase 1 — Compose Signature Change and Cleanup Plumbing

### Target file: `pkg/ui/compositor.go`

### Tasks

- [ ] **T-1.1**: Change `Compose` signature from `(fyne.CanvasObject, error)` to `(fyne.CanvasObject, func(), error)` at line 63. Update the function body to propagate the cleanup from `composeWithState`. On error path: return `nil, nil, err`. On success: return `obj, cleanup, nil`. (REQ-CLEANUP-01)

- [ ] **T-1.2**: Change `composeWithState` signature from `(fyne.CanvasObject, error)` to `(fyne.CanvasObject, func(), error)` at line 69. Update all `case` branches:

  - **`container`** (line 71): Recurse on `node.Children`, collecting each child's cleanup func into a `[]func()` slice. Return a single composed func that calls all child cleanups in forward order. On recursive error: return `nil, nil, err`. (REQ-CLEANUP-01)

  - **`label`** (line 103): Return `widget.NewLabel(node.Label), func() {}, nil`.

  - **`text_input`** (line 106): Return the existing entry widget, `func() {}`, `nil`.

  - **`text_area`** (line 117): Return the existing entry widget, `func() {}`, `nil`.

  - **`button`** (line 128): Return the existing button widget, `func() {}`, `nil`.

  - **`data_grid`** (line 142): After existing `model.unsubscribe` assignment (line 259) and `table.OnSelected` setup, create a `sync.Once`-wrapped cleanup func:
    ```go
    var once sync.Once
    cleanup := func() {
        once.Do(func() {
            model.mu.Lock()
            cancelFn := model.cancel
            unsubFn := model.unsubscribe
            model.cancel = nil
            model.unsubscribe = nil
            model.mu.Unlock()
            if cancelFn != nil { cancelFn() }
            if unsubFn != nil { unsubFn() }
        })
    }
    ```
    Return `table, cleanup, nil`. (REQ-CLEANUP-01, REQ-CLEANUP-05)

- [ ] **T-1.3**: Add `"sync"` import to `pkg/ui/compositor.go` (for `sync.Once`).

### Dependencies

T-1.3 must come first (import). T-1.1 and T-1.2 are a single atomic change (signature + all branches). T-1.1 depends on T-1.2.

### Verification

```bash
# Will not compile until Phase 6 migrates callers, so verify syntax only:
go vet ./pkg/ui/...
```

**Note:** Compilation will fail until Phase 6 updates all `Compose` call sites. This is expected — Phase 1 and Phase 6 should be committed together.

---

## 3. Phase 2 — Navigate Teardown (main.go)

### Target file: `cmd/golemui/main.go`

### Tasks

- [ ] **T-2.1**: Add `var prevCleanup func()` before the `ui.Navigate` closure assignment (before line 101).

- [ ] **T-2.2**: Inside `ui.Navigate`'s `go func()` block (line 103), add teardown at the top of the goroutine, before `LoadScreen`:
  ```go
  if prevCleanup != nil {
      prevCleanup()
      prevCleanup = nil
  }
  ```
  (REQ-CLEANUP-02)

- [ ] **T-2.3**: Update the `ui.Compose` call at line 108 to capture 3 return values:
  ```go
  newUI, cleanup, err := ui.Compose(node, vID)
  ```

- [ ] **T-2.4**: After the `Compose` error check and before `fyne.Do`, assign the new cleanup:
  ```go
  prevCleanup = cleanup
  ```

- [ ] **T-2.5**: Update the bootstrap `Compose` call at line 130 to capture 3 return values:
  ```go
  homeUI, homeCleanup, err := ui.Compose(homeNode, vistaID)
  ```
  After the error check, store: `prevCleanup = homeCleanup`. (REQ-CLEANUP-02)

### Dependencies

Depends on Phase 1 (Compose returns 3 values). Must be committed atomically with Phase 1 and Phase 6.

### Verification

```bash
go build ./cmd/golemui/...
```

---

## 4. Phase 3 — EventBus Test Helper

### Target file: `pkg/eventbus/eventbus_test.go`

### Tasks

- [ ] **T-3.1**: Add a `SubscriberCount` test helper function:
  ```go
  func SubscriberCount(t testing.TB, bus eventbus.EventBus, channel string) int {
      t.Helper()
      b, ok := bus.(*eventbus.InMemEventBus)
      if !ok {
          t.Fatalf("expected *InMemEventBus, got %T", bus)
      }
      b.mu.RLock()
      defer b.mu.RUnlock()
      return len(b.subscribers[channel])
  }
  ```
  (REQ-CLEANUP-03 — enables test assertions on subscriber count)

### Dependencies

None. Can be done in parallel with Phase 1.

### Verification

```bash
go test ./pkg/eventbus/... -count=1
```

---

## 5. Phase 4 — New TDD Tests (compositor_test.go)

### Target file: `pkg/ui/compositor_test.go`

### Tasks

- [ ] **T-4.1**: `TestCompose_ReturnsCleanupFunc` — Compose a screen with one `data_grid`. Assert `cleanup != nil`. Invoke cleanup, assert no panic. (TDD-01, REQ-CLEANUP-01)

- [ ] **T-4.2**: `TestCompose_CleanupRemovesSubscribers` — Compose a `data_grid`, verify `SubscriberCount(t, bus, "screen:submit:test-vista") == 1`. Call cleanup. Verify `SubscriberCount` drops to 0. Publish on the old channel and assert zero handler invocations (use `atomic.Int32` spy). (TDD-02, REQ-CLEANUP-03)

- [ ] **T-4.3**: `TestCompose_CleanupCancelsGoroutines` — Compose a client-mode `data_grid` with a mock pool whose `Query` blocks on an unbuffered channel. Call cleanup. Assert the blocked goroutine terminates within 1 second (via `runtime.NumGoroutine()` delta or `sync.WaitGroup`). (TDD-03, REQ-CLEANUP-04)

- [ ] **T-4.4**: `TestCompose_IdempotentCleanup` — Compose a `data_grid`. Call cleanup twice. Assert no panic, subscriber count stays at 0 after both calls. (TDD-05, REQ-CLEANUP-05)

- [ ] **T-4.5**: `TestCompose_NoOpCleanup_NoDataGrid` — Compose a label-only screen (`container` + `label`). Assert `cleanup != nil` (non-nil no-op). Call cleanup, assert no panic. (TDD-06, REQ-CLEANUP-01)

### Dependencies

Depends on Phase 1 (Compose returns 3 values) and Phase 3 (SubscriberCount helper).

### Verification

```bash
go test ./pkg/ui/... -count=1 -v -run "TestCompose_Returns|TestCompose_Cleanup|TestCompose_Idempotent|TestCompose_NoOp"
```

---

## 6. Phase 5 — Navigate TDD Test (main_test.go)

### Target file: `cmd/golemui/main_test.go`

### Tasks

- [ ] **T-5.1**: `TestNavigate_TearsDownPreviousScreen` — Use `RunBootstrap` with a home screen containing a `data_grid`. Capture the `LocalEventBus` before navigation. Register a second screen layout in the mock pool. Call `ui.Navigate("screen-b")`. Wait for async navigation (poll with timeout). Assert `SubscriberCount(t, bus, "screen:submit:home") == 0`. (TDD-04, REQ-CLEANUP-02)

### Dependencies

Depends on Phase 1, Phase 2, Phase 3, and Phase 6.

### Verification

```bash
go test ./cmd/golemui/... -count=1 -v -run "TestNavigate_TearsDown"
```

---

## 7. Phase 6 — Migrate Existing Compose Callers

### Target files

1. `pkg/ui/compositor_test.go` — 22 call sites (lines 45, 85, 119, 192, 254, 282, 363, 484, 519, 573, 645, 719, 818, 920, 940, 1017, 1102, 1149, 1207, 1285, 1338, 1384)
2. `cmd/golemui/main.go` — 2 call sites (lines 108, 130; handled by Phase 2)

### Tasks

- [ ] **T-6.1**: Update all 22 `ui.Compose(node, vistaID)` calls in `compositor_test.go` from:
  ```go
  obj, err := ui.Compose(node, "test-vista")
  ```
  to:
  ```go
  obj, cleanup, err := ui.Compose(node, "test-vista")
  if err != nil { /* existing error handling */ }
  defer cleanup()
  ```
  Each call site is a mechanical 2-line change. The error handling pattern varies slightly per test (some use `t.Fatal`, some continue) — preserve the existing error handling and insert `defer cleanup()` after the error check. (REQ-CLEANUP-07)

### Dependencies

Depends on Phase 1 (new signature). Must be committed atomically with Phase 1 and Phase 2.

### Verification

```bash
go build ./...
go test ./... -count=1 -timeout 60s
```

---

## 8. Phase 7 — Final Verification

### Tasks

- [ ] **T-7.1**: Run full build: `go build ./...`
- [ ] **T-7.2**: Run vet: `go vet ./...`
- [ ] **T-7.3**: Run full test suite: `go test ./... -count=1 -timeout 60s`
- [ ] **T-7.4**: Grep audit — verify zero diff in `pkg/eventbus/eventbus.go`:
  ```bash
  git diff pkg/eventbus/eventbus.go
  # Expected: no changes
  ```
- [ ] **T-7.5**: Grep audit — verify all `ui.Compose` calls handle 3 return values:
  ```bash
  grep -rn "ui\.Compose" --include="*.go" | grep -v "_test.go" | grep -v "func "
  # Expected: only the two call sites in main.go, both with 3-value assignment
  ```

### Dependencies

All prior phases complete.

---

## 9. Implementation Order

```
Phase 3 (EventBus helper)          ← independent, can start immediately
  │
Phase 1 (Compose signature)
  │
  ├── Phase 6 (migrate test callers) ── must be atomic with Phase 1
  │
  ├── Phase 2 (Navigate teardown)   ── must be atomic with Phase 1
  │
  └── Phase 4 (new TDD tests)       ── after Phase 1 + Phase 3
       │
       └── Phase 5 (Navigate test)  ── after Phase 2 + Phase 6
            │
            └── Phase 7 (verification)
```

**Commit strategy:** Phases 1 + 2 + 6 are one atomic commit (signature change + all callers). Phase 3 is an independent commit. Phase 4 + 5 are a second commit (tests). Phase 7 is verification only.

---

## 10. Estimated Changed Lines per Task

| Task | File | Lines changed |
|------|------|--------------|
| T-1.1 | `compositor.go` | ~+4, ~-2 |
| T-1.2 | `compositor.go` | ~+20, ~-3 |
| T-1.3 | `compositor.go` | +1 (import) |
| T-2.1–T-2.5 | `main.go` | ~+10, ~-4 |
| T-3.1 | `eventbus_test.go` | ~+12 |
| T-4.1 | `compositor_test.go` | ~+15 |
| T-4.2 | `compositor_test.go` | ~+25 |
| T-4.3 | `compositor_test.go` | ~+25 |
| T-4.4 | `compositor_test.go` | ~+10 |
| T-4.5 | `compositor_test.go` | ~+15 |
| T-5.1 | `main_test.go` | ~+30 |
| T-6.1 | `compositor_test.go` | ~+22 (1 line per call site) |
| T-7.x | CLI only | 0 |
| **Total** | | **~+190 lines** |
