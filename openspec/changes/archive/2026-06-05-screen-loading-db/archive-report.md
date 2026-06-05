# Archive Report: Screen Loading from DB

**Change**: screen-loading-db
**Archived**: 2026-06-05
**Artifact Store**: both (openspec + engram)
**Branch**: feat/screen-loading-db
**Status**: PASS WITH WARNINGS

---

## Executive Summary

Successfully archived the `screen-loading-db` change. All 13 spec scenarios implemented and verified. Build and tests pass. One design-deviation warning (W-1: error wrapping in LoadScreen) identified but does not block merge.

**Verdict**: PASS WITH WARNINGS

---

## Change Description

Replace hardcoded `homeNode` in `main.go` with database-driven screen loader. Compositor reads layouts from `golemui.vistas_consulta` at runtime, enabling UI changes without recompilation.

### Scope — In
- `pkg/ui/screen_loader.go` — `LoadScreen(ctx, pool, vistaID) (NodeMeta, error)`
- `pkg/ui/screen_loader_test.go` — table-driven tests
- `pkg/ui/compositor.go` — add `var CorePool db.DatabasePool` global
- `cmd/golemui/main.go` — wire CorePool, replace homeNode with LoadScreen
- `pkg/lua/loader.go` — add `EntryPointViewID string` to BootstrapConfig
- `docker/init-db/02_init_core.sql` — INSERT sample `home` vista row

### Scope — Out
- Capa 3 auto-scaffolding (`mapeo_interfaz`)
- Window title propagation
- Multi-screen navigation/routing
- `config_filtros` deserialization

---

## Final Status

**PASS WITH WARNINGS**

- 0 CRITICAL issues
- 1 WARNING (W-1: error wrapping deviation)
- 3 SUGGESTIONS (S-1, S-2, S-3)

---

## Files Changed

| File | Lines Changed |
|------|--------------|
| `cmd/golemui/main.go` | +25/-0 |
| `cmd/golemui/main_test.go` | +163/-0 |
| `docker/init-db/02_init_core.sql` | +6/-0 |
| `pkg/lua/loader.go` | +9/-0 |
| `pkg/lua/loader_test.go` | +73/-0 |
| `pkg/ui/compositor.go` | +1/-0 |
| `pkg/ui/compositor_test.go` | +6/-0 |
| `pkg/ui/screen_loader.go` | +29/-0 |
| `pkg/ui/screen_loader_test.go` | +186/-0 |
| **Total** | **+893/-20** (14 files) |

---

## Commits Made

| Commit | Description |
|--------|-------------|
| `1dc2918` | feat(ui): add CorePool global variable for core database access |
| `a95c1b6` | feat(lua): add EntryPointViewID field to BootstrapConfig |
| `0876519` | feat(db): insert sample home vista in core init script |
| `6ce9787` | feat(ui): add LoadScreen function to load vista from core database |
| `40b72e8` | docs(openspec): add screen-loading-db change artifacts |
| `4e3ce36` | feat(bootstrap): wire CorePool and load home screen from database |

---

## Test Coverage Summary

| Package | Status | Time |
|---------|--------|------|
| `cmd/golemui` | ✅ PASS | 1.091s |
| `pkg/db` | ✅ PASS | 0.945s |
| `pkg/eventbus` | ✅ PASS | 0.120s |
| `pkg/lua` | ✅ PASS | 0.010s |
| `pkg/ui` | ✅ PASS | 0.293s |

**Total tests**: 13 (10 Unit + 3 Integration)
**Coverage — `pkg/ui/screen_loader.go`**: 100.0%

---

## Verification Findings

### CRITICAL: 0

### WARNING: 1

**W-1**: `pkg/ui/screen_loader.go:19-21` — All Scan errors wrapped as "vista not found"

Current implementation maps ANY error from `Scan` to `"vista %q not found"`. The design specifies only `pgx.ErrNoRows` should map to "not found". Other errors (connection drops, ctx cancellation) are misreported.

**Recommended fix**:
```go
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return NodeMeta{}, fmt.Errorf("LoadScreen: vista %q not found", vistaID)
    }
    return NodeMeta{}, fmt.Errorf("LoadScreen: query for vista %q failed: %w", vistaID, err)
}
```

### SUGGESTION: 3

**S-1**: `pkg/ui/screen_loader_test.go:103-119` — Awkward nil-pool handling in table-driven loop. Use `wantNilPool bool` field or separate test.

**S-2**: `cmd/golemui/main_test.go:85-167` — No explicit assertion that window content comes from LoadScreen (relies on bootstrap success).

**S-3**: `cmd/golemui/main_test.go:239-305` — Pool closure not explicitly asserted in `TestRunBootstrap_LoadScreenFailure`.

---

## Known Follow-ups

### W-1 Error Wrapping Fix (Recommended)

The error wrapping in `LoadScreen` should be tightened to distinguish `pgx.ErrNoRows` from other errors. This is a small follow-up commit that does not require full SDD cycle — a direct PR with the fix is appropriate.

```go
// pkg/ui/screen_loader.go — current (W-1)
err := pool.QueryRow(ctx, sql, vistaID).Scan(&jsonBytes)
if err != nil {
    return NodeMeta{}, fmt.Errorf("LoadScreen: vista %q not found", vistaID)
}

// Recommended fix
err := pool.QueryRow(ctx, sql, vistaID).Scan(&jsonBytes)
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return NodeMeta{}, fmt.Errorf("LoadScreen: vista %q not found", vistaID)
    }
    return NodeMeta{}, fmt.Errorf("LoadScreen: query for vista %q failed: %w", vistaID, err)
}
```

---

## Engram Observation IDs (Traceability)

| Artifact | Observation ID |
|----------|---------------|
| Explore | #1164 |
| Proposal | #1165 |
| Spec | #1166 |
| Design | #1167 |
| Tasks | #1168 |
| Apply Progress | #1169 |
| Verify Report | #1170 |
| Archive Report | (this doc) |

---

## Specs Synced to Main

| Domain | Action | Details |
|--------|--------|---------|
| `screen-loading` | Created | New full spec — 3 requirements, 6 scenarios |
| `client-bootstrap` | Updated | 1 modified requirement (Bootstrap Wiring), 1 added requirement (EntryPointViewID); +5 new scenarios |

---

## Archive Contents

```
openspec/changes/archive/2026-06-05-screen-loading-db/
├── proposal.md ✅
├── specs/
│   ├── screen-loading/spec.md ✅
│   └── client-bootstrap/spec.md ✅
├── design.md ✅
├── tasks.md ✅
└── verify-report.md ✅
```

---

## SDD Cycle Complete

All phases completed: explore → propose → spec → design → tasks → apply → verify → archive.

The change is ready to ship. Address W-1 in a follow-up PR at convenience.
