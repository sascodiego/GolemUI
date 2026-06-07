# Design: CLI Flags for GolemUI Bootstrap

## Technical Approach

Add stdlib `flag` package CLI flags (`-config`, `-view`) to `cmd/golemui/main.go`. The `-config` flag replaces the hardcoded `"golemui_driver.lua"` string in `main()`. The `-view` flag introduces a view override passed as a new 5th positional parameter `viewOverride string` to `RunBootstrap`. Inside `RunBootstrap`, the view ID resolves through a three-level override chain: `viewOverride` → `cfg.EntryPointViewID` → `"home"`. This maps directly to the proposal's Option A approach and satisfies all 3 delta spec requirements.

## Architecture Decisions

| Decision | Choice | Alternative | Rationale |
|----------|--------|-------------|-----------|
| Flag declaration style | Package-level `var` with `flag.String()` | Struct + `flag.Var()` | Idiomatic Go for ≤3 flags; keeps declaration and usage co-located. No custom `flag.Value` needed for plain strings. |
| `viewOverride` passing | 5th positional `string` param | Options struct (`BootstrapOpts`) | Scope constraint: one new param. An options struct is premature abstraction — the proposal explicitly deferred it. If params grow beyond 6, refactor then. |
| `flag.Parse()` location | `main()` only | Inside `RunBootstrap` | `flag.Parse()` mutates global `flag.CommandLine`. Placing it in `RunBootstrap` would poison test isolation — tests would need to reset or mock `flag.CommandLine`. By keeping it in `main()`, `RunBootstrap` stays a pure function of its arguments. |
| Override chain order | `viewOverride` → `cfg.EntryPointViewID` → `"home"` | Config wins over CLI | CLI flag is the operator's explicit intent; config is the developer's default. Operator intent always wins. Same precedence as `cfg.LayoutQuery` vs `ui.DefaultLayoutQuery`. |

## Data Flow

    CLI invocation
         │
         ▼
    main() ─── flag.Parse() ──→ *configPath, *viewOverride
         │
         ▼
    RunBootstrap(ctx, *configPath, runWindow, fyneApp, *viewOverride)
         │
         ├─ lua.LoadConfig(configPath) ──→ cfg
         │
         ▼
    vistaID resolution:
         viewOverride != "" ? ──yes──→ viewOverride
                              │
                              no
                              ▼
         cfg.EntryPointViewID != "" ? ──yes──→ cfg.EntryPointViewID
                                      │
                                      no
                                      ▼
                               "home"
         │
         ▼
    ui.LoadScreen(ctx, pool, vistaID, cfg.LayoutQuery)

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/golemui/main.go` | Modify | Add `"flag"` import; declare `configPath` and `viewOverride` flag vars; call `flag.Parse()` in `main()` before `RunBootstrap`; add `viewOverride string` as 5th param to `RunBootstrap`; replace lines 108-111 with override chain |
| `cmd/golemui/main_test.go` | Modify | Update 6 `RunBootstrap` call sites to pass `""` as 5th arg; add 3 new test functions |

### Detailed Changes per File

#### `cmd/golemui/main.go`

**Lines 1-16 (imports)**: Add `"flag"` to import block.

**After line 26 (package-level vars)**: Insert two flag declarations:
```go
var (
	flagConfigPath = flag.String("config", "golemui_driver.lua", "path to Lua configuration driver file")
	flagView       = flag.String("view", "", "override initial view ID (empty = use config default)")
)
```

**Line 47 (RunBootstrap signature)**: Add 5th parameter:
```go
func RunBootstrap(ctx context.Context, configPath string, runWindow bool, fyneApp fyne.App, viewOverride string) (*App, error) {
```

**Lines 108-111 (view resolution)**: Replace with override chain:
```go
vistaID := viewOverride
if vistaID == "" {
	vistaID = cfg.EntryPointViewID
}
if vistaID == "" {
	vistaID = "home"
}
```

**Lines 142-149 (main function)**: Add `flag.Parse()` and pass dereferenced flags:
```go
func main() {
	ctx := context.Background()
	flag.Parse()
	log.Println("Starting GolemUI desktop client bootstrap...")
	_, err := RunBootstrap(ctx, *flagConfigPath, true, nil, *flagView)
	if err != nil {
		log.Fatalf("Bootstrap error: %v", err)
	}
}
```

#### `cmd/golemui/main_test.go`

**6 existing call sites** — all change from 4 args to 5 args by appending `""`:

| Test function | Line | Current call | Updated call |
|---------------|------|-------------|-------------|
| `TestRunBootstrap_MissingConfig` | 102 | `RunBootstrap(ctx, "non_existent...", false, nil)` | `+ , "")` |
| `TestRunBootstrap_DatabaseFailure` | 137 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |
| `TestRunBootstrap_InvalidLuaConfigTable` | 163 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |
| `TestRunBootstrap_Success` | 225 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |
| `TestRunBootstrap_DefaultVistaID` | 309 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |
| `TestRunBootstrap_LoadScreenFailure` | 377 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |
| `TestRunBootstrap_IntegrationWithLogs` | 459 | `RunBootstrap(ctx, tmpFile, false, testApp)` | `+ , "")` |

Note: 7 call sites, not 6 — exploration said 6 but the actual file has 7. All must be updated atomically.

**3 new test functions** (appended after existing tests):

1. `TestRunBootstrap_ViewOverrideWins` — config has `EntryPointViewID = "dashboard"`, passes `viewOverride = "settings"`, asserts `LoadScreen` receives `"settings"`.
2. `TestRunBootstrap_EmptyOverrideFallsThrough` — config has `EntryPointViewID = "transacciones_list"`, passes `viewOverride = ""`, asserts view is `"transacciones_list"`.
3. `TestRunBootstrap_BothEmptyDefaultsHome` — config has no `EntryPointViewID`, passes `viewOverride = ""`, asserts view is `"home"`.

All three follow the existing mock pattern: swap `initDB`, create temp Lua, register `coreMock.RegisterQuery` for the expected view's `DefaultLayoutQuery`.

## Interfaces / Contracts

No new types or interfaces. The only contract change is the `RunBootstrap` signature:

```go
// Before
func RunBootstrap(ctx context.Context, configPath string, runWindow bool, fyneApp fyne.App) (*App, error)

// After
func RunBootstrap(ctx context.Context, configPath string, runWindow bool, fyneApp fyne.App, viewOverride string) (*App, error)
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Override chain: CLI wins over config | `TestRunBootstrap_ViewOverrideWins` — mock DB, pass `viewOverride="settings"`, register query for `"settings"`, verify no error |
| Unit | Override chain: empty CLI falls to config | `TestRunBootstrap_EmptyOverrideFallsThrough` — config sets `EntryPointViewID="transacciones_list"`, pass `""`, verify uses config value |
| Unit | Override chain: both empty → "home" | `TestRunBootstrap_BothEmptyDefaultsHome` — no `EntryPointViewID` in config, pass `""`, register query for `"home"`, verify success |
| Regression | 7 existing tests pass with 5th arg | Update all call sites to `RunBootstrap(..., "")`, verify green |

TDD order: RED (write 3 new tests — won't compile), GREEN (signature change + override chain + update 7 sites), REFACTOR (verify all pass).

## Migration / Rollout

No migration required. The change is backward compatible: default flag values produce byte-identical behavior to the current hardcoded values.

Rollback: revert the single commit — remove 5th param, restore hardcoded `"golemui_driver.lua"` in `main()`, drop flag vars.

## Open Questions

- None. All decisions are resolved within scope.
