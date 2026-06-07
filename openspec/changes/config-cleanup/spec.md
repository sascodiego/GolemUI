# Spec: config-cleanup

**Change ID:** config-cleanup  
**Type:** Pure refactor (rename + dead field removal)  
**Status:** draft  

## Summary

Rename `pkg/lua` → `pkg/config` and remove the dead `EntryPointQuery` field from `BootstrapConfig`. No behaviour changes; no new features.

## Scope

| Area | Files |
|------|-------|
| Package rename | `pkg/lua/loader.go` → `pkg/config/loader.go`, `pkg/lua/loader_test.go` → `pkg/config/loader_test.go` |
| Import updates | `cmd/golemui/main.go`, `cmd/golemui/main_test.go`, `pkg/config/loader_test.go` |
| Dead field removal | `pkg/config/loader.go` (struct), all test YAML fixtures, assertion blocks |

## Requirements

| ID | Requirement |
|----|-------------|
| REQ-001 | Directory `pkg/lua/` renamed to `pkg/config/`; `package` declarations updated from `lua` / `lua_test` to `config` / `config_test`. |
| REQ-002 | All Go import paths `"GolemUI/pkg/lua"` replaced with `"GolemUI/pkg/config"`. |
| REQ-003 | All `lua.` qualified prefixes updated to `config.` for `BootstrapConfig`, `ConfigConexion`, and `LoadConfig`. |
| REQ-004 | `EntryPointQuery` field removed from `BootstrapConfig` struct and its `mapstructure:"entry_point_query"` tag. |
| REQ-005 | All `entry_point_query` YAML lines removed from inline test fixtures across both test files. |
| REQ-006 | `EntryPointQuery` assertion block removed from `TestLoadConfig_Success`. |
| REQ-007 | All tests pass: `go test ./...` exits 0. |
| REQ-008 | Binary compiles: `go build ./...` exits 0. |
| REQ-009 | Zero remaining references to `"pkg/lua"` in `*.go` files (`grep -rn "pkg/lua" --include="*.go"` returns empty). |

## Out of Scope

- No change to `EntryPointViewID` or `LayoutQuery` fields.
- No change to `cmd/golemui/main.go` bootstrap logic beyond import/prefix rename.
- No schema or database changes.
- No new tests; existing coverage must remain green.

## Scenarios

### SC-001: Build succeeds after rename

**Given** the directory `pkg/lua/` has been renamed to `pkg/config/`  
**When** `go build ./...` is executed  
**Then** the build completes with exit code 0 and all import paths resolve.

### SC-002: EntryPointQuery is fully removed

**Given** the `EntryPointQuery` field is removed from `BootstrapConfig`  
**When** searching the codebase with `grep -rn "EntryPointQuery" --include="*.go"` and `grep -rn "entry_point_query" --include="*.go"`  
**Then** both searches return zero matches.  
**And** all inline YAML fixtures in test files contain no `entry_point_query` lines.

### SC-003: All tests pass with new package name

**Given** all renames and removals are applied  
**When** `go test ./...` is executed  
**Then** all tests pass (exit code 0) with no failures related to the package rename or removed field.

## Acceptance Criteria

1. `go build ./...` — green.
2. `go test ./...` — green.
3. `grep -rn "pkg/lua" --include="*.go"` — zero matches.
4. `grep -rn "EntryPointQuery" --include="*.go"` — zero matches.
5. `grep -rn "entry_point_query" --include="*.go"` — zero matches.

## Risks

| Risk | Mitigation |
|------|------------|
| Missed import in less-visible files | REQ-009 grep gate catches stragglers. |
| YAML fixture still references removed field | REQ-005 + SC-002 grep catches it; test would also fail on unmarshal. |

## Rollback

Pure file-system rename: `git mv pkg/config pkg/lua`, revert import changes. No schema migration involved.
