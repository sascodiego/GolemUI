# Tasks: code-quality-antipatterns

**Change ID:** `code-quality-antipatterns`
**Date:** 2026-06-07
**Status:** Applied
**Config:** `openspec/config.yaml` — strict TDD enabled

---

## Tasks

### §4.2 — Atomic Re-entrancy Guard (sidebar_widget.go)

- [x] T1: Add test `TestNavigating_InitialState` to `pkg/ui/sidebar_widget_test.go` (RED phase)
- [x] T2: Change `navigating bool` → `navigating atomic.Bool` in `NavTree` struct in `pkg/ui/sidebar_widget.go`
- [x] T3: Add `"sync/atomic"` import to `pkg/ui/sidebar_widget.go`
- [x] T4: Change `nt.navigating = true` → `nt.navigating.Store(true)` in `SelectByVistaID`
- [x] T5: Change `nt.navigating = false` → `nt.navigating.Store(false)` in `SelectByVistaID` defer
- [x] T6: Change `if navTree.navigating {` → `if navTree.navigating.Load() {` in `OnSelected`
- [x] T7: Verify all sidebar tests pass: `go test ./pkg/ui/ -count=1`

### §4.3 — Pool Cleanup + prevCleanup Mutex (main.go)

- [x] T8: Add `"sync"` import to `cmd/golemui/main.go`
- [x] T9: Add `cleanupMu sync.Mutex` variable alongside `prevCleanup`
- [x] T10: Wrap prevCleanup call+nil in Navigate goroutine with `cleanupMu.Lock()/Unlock()`
- [x] T11: Wrap prevCleanup assignment in Navigate goroutine with `cleanupMu.Lock()/Unlock()`
- [x] T12: Add `win.SetOnClosed()` callback before `win.ShowAndRun()` — calls prevCleanup under lock, then dbPool.Close() outside lock
- [x] T13: Verify all main tests pass: `go test ./cmd/golemui/ -count=1`

### Validation

- [x] T14: `go build ./...` exits 0
- [x] T15: `go test ./... -count=1` exits 0
- [x] T16: `go vet ./...` exits 0
- [x] T17: `gofmt -l .` exits 0 (no unformatted files)
- [x] T18: `go test -race ./pkg/ui/ -count=1` exits 0 for sidebar-specific tests (zero race warnings)
