# Tasks: CLI Flags for GolemUI Bootstrap

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~120 (30 new in main.go, 80 new tests, 7 modified in existing tests) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | auto-chain |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Full CLI flags feature (RED + GREEN + VERIFY) | PR 1 | Branch: feat/cli-flags-bootstrap; all tests included |

## Phase 1: RED — Write Failing Tests

- [x] 1.1 Add 3 new test functions to `cmd/golemui/main_test.go` (after line 463): `TestRunBootstrap_ViewOverrideWins`, `TestRunBootstrap_EmptyOverrideFallsThrough`, `TestRunBootstrap_BothEmptyDefaultsHome`. Each calls `RunBootstrap` with 5 args (viewOverride param). Won't compile — this IS the RED state.
- [x] 1.2 `TestRunBootstrap_ViewOverrideWins`: config sets `EntryPointViewID = "dashboard"`, pass `viewOverride = "settings"`, register mock query for "settings" layout, assert success.
- [x] 1.3 `TestRunBootstrap_EmptyOverrideFallsThrough`: config sets `EntryPointViewID = "transacciones_list"`, pass `""`, register mock query for "transacciones_list" layout, assert success.
- [x] 1.4 `TestRunBootstrap_BothEmptyDefaultsHome`: config has no `EntryPointViewID`, pass `""`, register mock query for "home" (DefaultLayoutQuery), assert success.

## Phase 2: GREEN — Signature Change & Existing Test Fix

- [x] 2.1 Add `"flag"` import to `cmd/golemui/main.go` imports (line 3-16).
- [x] 2.2 Insert package-level flag vars after line 26: `flagConfigPath` (default `"golemui_driver.lua"`) and `flagView` (default `""`).
- [x] 2.3 Change `RunBootstrap` signature at line 47 to add 5th param `viewOverride string`.
- [x] 2.4 Update all 7 existing `RunBootstrap` call sites in `main_test.go` to pass `""` as 5th arg (lines 102, 137, 163, 225, 309, 377, 459).

## Phase 3: GREEN — Override Chain & Flag Wiring

- [x] 3.1 Replace lines 108-111 in `cmd/golemui/main.go` with override chain: `viewOverride` → `cfg.EntryPointViewID` → `"home"`.
- [x] 3.2 Add `flag.Parse()` in `main()` before `RunBootstrap` call (line 142-149).
- [x] 3.3 Update `RunBootstrap` call in `main()` to pass `*flagConfigPath` and `*flagView`.

## Phase 4: VERIFY — Full Suite

- [x] 4.1 Run `go test ./cmd/golemui/...` — all 16 tests pass.
- [x] 4.2 Run `go build ./...` — must compile clean.
- [x] 4.3 Confirm 3 new tests exercise full override chain: CLI wins → config fallback → "home" default.
