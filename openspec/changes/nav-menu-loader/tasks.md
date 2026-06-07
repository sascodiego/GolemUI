# Tasks: nav-menu-loader

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~200 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |

## Implementation Tasks

### Task 1 — Create sidebar_loader.go with MenuItem struct

- [ ] Create `pkg/ui/sidebar_loader.go` with `MenuItem` struct, `NavigationMenuQuery` const, `LoadNavigationMenu` function, `validateNoCycles` function

**Estimated lines:** ~80

### Task 2 — Create sidebar_loader_test.go

- [ ] Create `pkg/ui/sidebar_loader_test.go` with `package ui_test` covering:
  - Valid acyclic hierarchy loads and sorts correctly
  - Cyclic hierarchy (A→B→A) returns error with "cycle detected"
  - Self-loop (X→X) returns error with cycle path
  - Nil pool returns "LoadNavigationMenu: pool is nil"
  - Empty result returns empty non-nil slice

**Estimated lines:** ~120

### Task 3 — Verify

- [ ] `go test ./pkg/ui/...` passes
- [ ] `go vet ./pkg/ui/...` clean
- [ ] No Fyne imports in sidebar_loader.go

---

## Summary

| Task | Est. Lines |
|------|------------|
| 1 | ~80 |
| 2 | ~120 |
| 3 | 0 |
| **Total** | **~200** |
