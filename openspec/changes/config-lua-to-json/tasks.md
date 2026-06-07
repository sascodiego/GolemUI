# Tasks: Migrate Bootstrap Config from Lua to JSON

Change: `config-lua-to-json`

## Overview

Replace the Lua-based config loader (`pkg/lua`) with a JSON-based loader. The package keeps its name (`pkg/lua`) and public API surface (`BootstrapConfig`, `ConfigConexion`, `LoadConfig`). The structs gain JSON tags, the implementation swaps `gopher-lua` VM for `os.ReadFile` + `encoding/json`, and all test fixtures switch from inline Lua to inline JSON strings. The CLI default flag changes from `.lua` to `.json`, and the repo ships a `golemui_driver.json` instead of `golemui_driver.lua`.

---

## Phase 1 — Struct Tags & Loader Core

### Task 1

| Field | Value |
|---|---|
| **ID** | T1 |
| **Description** | Add `json:"..."` struct tags to `ConfigConexion` and `BootstrapConfig`. Fields map to their Lua names (PascalCase → PascalCase in JSON). No behaviour change — this is tag-only. |
| **Files affected** | `pkg/lua/loader.go` |
| **Depends on** | — |
| **Estimated lines changed** | ~14 (two structs, one tag per field) |
| **Acceptance test** | `go vet ./pkg/lua/` passes; `go build ./pkg/lua/` compiles without error. Existing tests still pass. |

### Task 2

| Field | Value |
|---|---|
| **ID** | T2 |
| **Description** | Rewrite `LoadConfig` to use `os.ReadFile` + `json.Unmarshal` into `BootstrapConfig`. Add a `validateConexion` helper that checks `Host != "" && Port != 0 && Database != "" && User != ""` and returns a descriptive error. Remove `getStringField`, `getIntField`, the `gopher-lua` import, and the `fmt` import (if no longer used). The new `LoadConfig` must: (a) stat the file, (b) read it, (c) unmarshal, (d) validate UIDB, (e) validate BusinessDB. The outer JSON object is `BootstrapConfig` directly — no `golemui_driver` wrapper key. |
| **Files affected** | `pkg/lua/loader.go` |
| **Depends on** | T1 |
| **Estimated lines changed** | ~40 removed, ~25 added (net −15) |
| **Acceptance test** | `go build ./pkg/lua/` compiles. No reference to `lua "github.com/yuin/gopher-lua"` in the file. `grep -q 'gopher-lua' pkg/lua/loader.go` returns 1. A manual JSON config unmarshal + validation works when tested locally. |

---

## Phase 2 — Loader Tests

### Task 3

| Field | Value |
|---|---|
| **ID** | T3 |
| **Description** | Rewrite all 8 tests in `pkg/lua/loader_test.go`. Replace every inline Lua content string with the equivalent inline JSON. Test list (preserved from current suite): (1) `TestLoadConfig_MissingFile` — unchanged logic, just use `.json` extension. (2) `TestLoadConfig_Success` — JSON with full UIDB, BusinessDB, EntryPointQuery. (3) `TestLoadConfig_InvalidSyntax` — malformed JSON (e.g. missing brace/quote). (4) `TestLoadConfig_MissingFields` — JSON with Host omitted from UIDB; expect validation error. (5) `TestLoadConfig_EntryPointViewID_Present` — JSON including `EntryPointViewID`. (6) `TestLoadConfig_EntryPointViewID_Absent` — JSON without `EntryPointViewID`; expect empty string. (7) `TestLoadConfig_LayoutQuery_Present` — JSON including `LayoutQuery`. (8) `TestLoadConfig_LayoutQuery_Absent` — JSON without `LayoutQuery`; expect empty string. Error message assertions update from Lua-specific text to JSON/validation text where needed (e.g. `"missing required"` or `"failed to parse"`). |
| **Files affected** | `pkg/lua/loader_test.go` |
| **Depends on** | T2 |
| **Estimated lines changed** | ~120 replaced (same structure, JSON fixtures instead of Lua) |
| **Acceptance test** | `go test ./pkg/lua/ -v` — all 8 tests pass green. |

---

## Phase 3 — CLI Default & Config File Swap

### Task 4

| Field | Value |
|---|---|
| **ID** | T4 |
| **Description** | Update the CLI `-config` flag default from `"golemui_driver.lua"` to `"golemui_driver.json"`. Update the flag help text from "Path to Lua configuration file" to "Path to JSON configuration file". |
| **Files affected** | `cmd/golemui/main.go` |
| **Depends on** | T2 |
| **Estimated lines changed** | ~2 |
| **Acceptance test** | `grep -n 'golemui_driver.json' cmd/golemui/main.go` shows the updated default. `go build ./cmd/golemui/` compiles. Running the binary without `-config` flag prints a log line referencing `golemui_driver.json`. |

### Task 5

| Field | Value |
|---|---|
| **ID** | T5 |
| **Description** | Create `golemui_driver.json` at project root with the same values currently in `golemui_driver.lua`. JSON structure mirrors `BootstrapConfig` (no wrapper key). Delete `golemui_driver.lua`. |
| **Files affected** | `golemui_driver.json` (created), `golemui_driver.lua` (deleted) |
| **Depends on** | T1 (tags define the field names) |
| **Estimated lines changed** | +20 / −17 |
| **Acceptance test** | `cat golemui_driver.json | python3 -m json.tool` validates. `test -f golemui_driver.lua` returns non-zero. `grep -r 'golemui_driver.lua' --include='*.go' .` returns no results (T4 already changed the flag). |

---

## Phase 4 — Bootstrap Tests

### Task 6

| Field | Value |
|---|---|
| **ID** | T6 |
| **Description** | Rewrite all 9 bootstrap tests in `cmd/golemui/main_test.go` that embed inline Lua config strings. Replace each Lua content string with the equivalent JSON. Test list: (1) `TestRunBootstrap_MissingConfig` — update filename to `.json`. (2) `TestRunBootstrap_DatabaseFailure` — JSON with unreachable hosts. (3) `TestRunBootstrap_InvalidLuaConfigTable` → rename to `TestRunBootstrap_InvalidJSONConfig`; provide JSON missing required fields so `validateConexion` fires. Update error assertion from `"golemui_driver table not found"` to match the new validation/unmarshal error. (4) `TestRunBootstrap_Success` — full JSON config. (5) `TestRunBootstrap_DefaultVistaID` — JSON without `EntryPointViewID`. (6) `TestRunBootstrap_LoadScreenFailure` — JSON with `EntryPointViewID = "nonexistent"`. (7) `TestRunBootstrap_ViewOverrideWins` — JSON with `EntryPointViewID = "dashboard"`. (8) `TestRunBootstrap_EmptyOverrideFallsThrough` — JSON with `EntryPointViewID = "transacciones_list"`. (9) `TestRunBootstrap_BothEmptyDefaultsHome` — JSON without `EntryPointViewID`. Also update `TestRunBootstrap_IntegrationWithLogs` (10th test) if it embeds Lua content. Verify all test function names that reference "Lua" in their name are renamed to reference "JSON" or generic terms. |
| **Files affected** | `cmd/golemui/main_test.go` |
| **Depends on** | T2, T4 |
| **Estimated lines changed** | ~200 replaced (same test structure, JSON fixtures instead of Lua) |
| **Acceptance test** | `go test ./cmd/golemui/ -v -run TestRunBootstrap` — all bootstrap tests pass green. `grep -r 'golemui_driver = {' cmd/golemui/main_test.go` returns 0 matches. |

---

## Phase 5 — Final Verification

### Task 7

| Field | Value |
|---|---|
| **ID** | T7 |
| **Description** | Verify `gopher-lua` remains in `go.mod` and `go.sum` (no removal). Run the full test suite: `go test ./pkg/lua/ ./cmd/golemui/ -v`. Run `go vet ./...`. Confirm no remaining references to Lua config loading in production code (grep for `gopher-lua` in `pkg/lua/loader.go` should return 0; grep in `cmd/golemui/main.go` should return 0). The `pkg/lua` package name stays unchanged — only the implementation changes. |
| **Files affected** | None (verification only) |
| **Depends on** | T3, T4, T5, T6 |
| **Estimated lines changed** | 0 |
| **Acceptance test** | `go test ./pkg/lua/ ./cmd/golemui/ -v` all pass. `go vet ./...` clean. `grep 'gopher-lua' pkg/lua/loader.go` returns exit 1. `grep 'github.com/yuin/gopher-lua' go.mod` returns exit 0 (present). |

---

## Dependency Graph

```
T1 ──→ T2 ──→ T3 ──→ T7
  │      │      │
  │      ├──────┤
  │      │      └──→ (parallel with T6)
  │      │
  │      ├──→ T4 ──→ T6 ──→ T7
  │      │
  └──→ T5 ─────────────→ T7
```

**Recommended execution order:** T1 → T2 → T3 + T4 + T5 (parallel) → T6 → T7

## Summary

| Task | Phase | Lines (est.) | Risk |
|------|-------|-------------|------|
| T1 | Struct tags | ~14 | Low |
| T2 | Loader rewrite | ~65 | Medium — core logic change |
| T3 | Loader tests | ~120 | Low — mechanical fixture swap |
| T4 | CLI default | ~2 | Low |
| T5 | Config file swap | ~37 | Low |
| T6 | Bootstrap tests | ~200 | Medium — 10 tests, error message changes |
| T7 | Verification | 0 | None |
| **Total** | | **~438** | |
