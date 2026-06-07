# Proposal: Layout Query Decoupling & Locale Fix

## Intent

Two tightly-scoped fixes in a single change:

1. **Layout Query Decoupling**: The SQL query in `pkg/ui/screen_loader.go:20` is hardcoded with no path from configuration. Externalize it into a `LayoutQuery` field on `BootstrapConfig`, parsed from `golemui_driver.lua`, with a defensive fallback constant preserving backward compatibility.

2. **Locale Sanitization**: On systems where `LANG`/`LC_ALL` are unset or `C`/`POSIX`, Fyne emits `Error parsing user locale C` and keyboard layouts break. Sanitize these env vars before `app.New()`.

## Scope

### In Scope
- Add `LayoutQuery string` field to `BootstrapConfig` in `pkg/lua/loader.go`
- Parse via existing `getStringField(tbl, "LayoutQuery")`
- Add `DefaultLayoutQuery` constant in `pkg/ui/screen_loader.go` matching current hardcoded SQL
- Change `LoadScreen` signature to accept `layoutQuery string`; use injected value with fallback to `DefaultLayoutQuery` when empty
- Update `golemui_driver.lua` with `LayoutQuery` property
- Update 2 call sites in `cmd/golemui/main.go` (line 70 closure + line 89 direct call)
- Create `sanitizeLocale()` helper in `cmd/golemui/main.go`; call before `app.New()` (line 63)
- Inspect `LANG` and `LC_ALL`; if both absent/`C`/`POSIX`, force `os.Setenv("LANG", "en_US.UTF-8")`
- Update 9 test mock `RegisterQuery` calls across `screen_loader_test.go` and `main_test.go`
- Add new tests for `LayoutQuery` present/absent, fallback, and locale sanitization scenarios

### Out of Scope
- `pkg/ui/compositor.go` — no changes
- `EntryPointQuery` field — dead code, leave alone
- Env vars beyond `LANG`/`LC_ALL`
- SQL injection hardening (config author is already a privileged principal)
- Removing the dead `EntryPointQuery` field

## Capabilities

### New Capabilities
- `locale-sanitization`: Bootstrap-time sanitization of `LANG`/`LC_ALL` env vars to ensure Fyne locale parsing succeeds on systems with `C` or unset locales.

### Modified Capabilities
- `client-bootstrap`: Add `LayoutQuery` field to `BootstrapConfig`, parsed from Lua. The bootstrap sequence now passes the parsed query to `LoadScreen` call sites.
- `screen-loading`: `LoadScreen` signature gains a `layoutQuery string` parameter. When non-empty, it is used instead of the hardcoded SQL. A `DefaultLayoutQuery` constant provides the fallback.

## Approach

**Layout Query**: Follow the exact pattern of `EntryPointViewID` — a top-level string field parsed by `getStringField`. Pass the parsed value through both `LoadScreen` call sites (initial load + `ui.Navigate` closure). The `DefaultLayoutQuery` constant keeps all 9 existing test mocks working unchanged — they register the default query, and when `layoutQuery` is empty (as it will be in mocks that don't set `cfg.LayoutQuery`), the fallback matches.

**Locale**: A pure function `sanitizeLocale()` at the top of `RunBootstrap`, before `app.New()`. Checks `LANG` and `LC_ALL` via `os.Getenv`. If neither provides a valid locale (both empty, `C`, or `POSIX`), sets `LANG=en_US.UTF-8`. Tests use `t.Setenv` (auto-restored, no `t.Parallel()`).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `pkg/lua/loader.go` | Modified | Add `LayoutQuery` field + parsing |
| `pkg/ui/screen_loader.go` | Modified | New parameter + `DefaultLayoutQuery` constant |
| `cmd/golemui/main.go` | Modified | `sanitizeLocale()`, pass `cfg.LayoutQuery` to `LoadScreen` |
| `golemui_driver.lua` | Modified | Add `LayoutQuery` property |
| `pkg/lua/loader_test.go` | Modified | Add LayoutQuery present/absent tests |
| `pkg/ui/screen_loader_test.go` | Modified | Verify dynamic query + fallback behavior |
| `cmd/golemui/main_test.go` | Modified | Update mocks + add locale sanitization tests |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `LoadScreen` signature break misses a call site | Low | Exploration identified exactly 2 call sites (line 70 + 89) |
| Empty `LayoutQuery` + broken fallback = zero-length SQL | Low | `DefaultLayoutQuery` constant is hardcoded non-empty |
| Locale fix races in parallel tests | Low | Use `t.Setenv`, forbid `t.Parallel()` in locale tests |
| Fyne reads env before our sanitization | Low | `sanitizeLocale()` runs as first statement in `RunBootstrap`, before `app.New()` |

## Rollback Plan

1. Revert the feature branch — all changes are confined to 7 files
2. The `DefaultLayoutQuery` constant equals the current hardcoded string, so reverting restores identical behavior
3. No database schema changes — no rollback needed for persistence layer

## Dependencies

- None beyond existing codebase (Go stdlib `os`, existing Lua parser, existing `pgx` pool)

## Success Criteria

- [ ] `go test ./...` passes with no regressions
- [ ] `LayoutQuery` field parsed from Lua; when set, `LoadScreen` uses it instead of default
- [ ] When `LayoutQuery` is empty/missing, `LoadScreen` falls back to `DefaultLayoutQuery`
- [ ] `sanitizeLocale()` sets `LANG=en_US.UTF-8` when both `LANG` and `LC_ALL` are absent/`C`/`POSIX`
- [ ] Valid locales (e.g. `es_ES.UTF-8`) are left untouched
- [ ] `golemui_driver.lua` contains `LayoutQuery` matching current production SQL
