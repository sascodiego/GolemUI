# Tasks: Layout Query Decoupling & Locale Fix

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~200 (additions ~165, modifications ~35) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | auto-forecast |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Full change (layout decoupling + locale fix) | PR 1 | Single PR; ~200 lines; all tests green |

## Phase 1: LayoutQuery Parsing in BootstrapConfig (TDD)

- [x] 1.1 RED: Write `TestLoadConfig_LayoutQuery_Present` in `pkg/lua/loader_test.go` — temp Lua with `LayoutQuery = "SELECT col FROM tbl"`, assert `cfg.LayoutQuery` equals it. **Fails**: field doesn't exist.
- [x] 1.2 GREEN: Add `LayoutQuery string` field to `BootstrapConfig` in `pkg/lua/loader.go:21`. Add `getStringField(tbl, "LayoutQuery")` call after line 104. Populate in return struct.
- [x] 1.3 RED: Write `TestLoadConfig_LayoutQuery_Absent` in `pkg/lua/loader_test.go` — Lua config without `LayoutQuery` key, assert `cfg.LayoutQuery == ""`. Passes immediately (GREEN already from 1.2).

## Phase 2: DefaultLayoutQuery Constant & LoadScreen Refactor (TDD)

- [x] 2.1 RED: Write `TestDefaultLayoutQuery` in `pkg/ui/screen_loader_test.go` — assert `ui.DefaultLayoutQuery == "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"`. **Fails**: constant doesn't exist.
- [x] 2.2 GREEN: Add `const DefaultLayoutQuery` to `pkg/ui/screen_loader.go` (line ~12).
- [x] 2.3 RED: Write `TestLoadScreen_CustomQuery` in `pkg/ui/screen_loader_test.go` — mock registered with custom SQL, call `LoadScreen(ctx, pool, "custom", customSQL)`, assert it uses custom query. **Fails**: signature only accepts 3 params.
- [x] 2.4 RED: Write `TestLoadScreen_EmptyFallback` — call `LoadScreen(ctx, pool, "home", "")`, assert uses `DefaultLayoutQuery`. **Fails**: same signature issue.
- [x] 2.5 GREEN: Change `LoadScreen` signature to 4 params in `pkg/ui/screen_loader.go`. Add fallback: `if layoutQuery == "" { layoutQuery = DefaultLayoutQuery }`. Remove hardcoded SQL on line 20.

## Phase 3: Test Mock Refactoring (REFACTOR)

- [x] 3.1 Replace all 5 hardcoded SQL strings in `pkg/ui/screen_loader_test.go` RegisterQuery calls with `ui.DefaultLayoutQuery`. Add `""` 4th arg to all `LoadScreen` calls.
- [x] 3.2 Replace all 4 hardcoded SQL strings in `cmd/golemui/main_test.go` RegisterQuery calls with `ui.DefaultLayoutQuery`.

## Phase 4: Wire LayoutQuery Through Bootstrap

- [x] 4.1 In `cmd/golemui/main.go`: pass `cfg.LayoutQuery` to `LoadScreen` at line 70 (Navigate closure) and line 89 (initial load). Add `"GolemUI/pkg/ui"` import if needed.

## Phase 5: Lua Driver Update

- [x] 5.1 Add `LayoutQuery = "SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1"` to `golemui_driver.lua` table.

## Phase 6: Locale Sanitization (TDD)

- [x] 6.1 RED: Write `TestSanitizeLocale_LangC` in `cmd/golemui/main_test.go` — `t.Setenv("LANG", "C")`, call `sanitizeLocale()`, assert `os.Getenv("LANG") == "en_US.UTF-8"`. **Fails**: function doesn't exist.
- [x] 6.2 RED: Write `TestSanitizeLocale_LCAllPOSIX` — `t.Setenv("LC_ALL", "POSIX")`, same assert. **Fails**.
- [x] 6.3 RED: Write `TestSanitizeLocale_BothEmpty` — unset both, assert `LANG == "en_US.UTF-8"`. **Fails**.
- [x] 6.4 RED: Write `TestSanitizeLocale_ValidLangUntouched` — `t.Setenv("LANG", "es_AR.UTF-8")`, assert unchanged. **Fails**.
- [x] 6.5 RED: Write `TestSanitizeLocale_LCAllValid` — `t.Setenv("LC_ALL", "en_US.UTF-8")`, assert `LANG == "en_US.UTF-8"`. **Fails**.
- [x] 6.6 GREEN: Implement `sanitizeLocale()` in `cmd/golemui/main.go`. Add `"os"` and `"strings"` imports. Function: inspect `LANG`/`LC_ALL`, apply rules per spec.
- [x] 6.7 WIRE: Call `sanitizeLocale()` as first line of `RunBootstrap` (before `lua.LoadConfig`).

## Phase 7: Integration Verification

- [x] 7.1 Run `go test ./...` — all tests green.
