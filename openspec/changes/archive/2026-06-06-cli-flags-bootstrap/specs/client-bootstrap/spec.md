# Delta for client-bootstrap

## ADDED Requirements

### Requirement: CLI Flag Declaration and Parsing

The `main()` function in `cmd/golemui/main.go` MUST declare two CLI flags using the stdlib `flag` package:

- `-config` (`string`, default `"golemui_driver.lua"`) — path to the Lua configuration driver file
- `-view` (`string`, default `""`) — optional override for the initial view ID

`flag.Parse()` MUST be called in `main()` before invoking `RunBootstrap`. Parsed values MUST be passed as arguments; `RunBootstrap` itself SHALL remain CLI-agnostic — no `flag` package dependency.

#### Scenario: Custom config path via `-config` flag

- GIVEN a valid Lua config file at `/tmp/custom_config.lua`
- WHEN `main()` is invoked with `-config=/tmp/custom_config.lua`
- THEN `RunBootstrap` SHALL receive `/tmp/custom_config.lua` as `configPath`
- AND bootstrap proceeds using that file

#### Scenario: Missing config file at custom path

- GIVEN no file exists at `/tmp/missing.lua`
- WHEN `main()` is invoked with `-config=/tmp/missing.lua`
- THEN `RunBootstrap` SHALL return a file-not-found error

#### Scenario: No flags provided — defaults applied

- GIVEN no CLI flags are passed
- WHEN `main()` is invoked
- THEN `configPath` SHALL default to `"golemui_driver.lua"`
- AND `viewOverride` SHALL default to `""`

### Requirement: View Override Resolution Chain

`RunBootstrap` MUST accept a `viewOverride string` parameter. The initial view ID SHALL be resolved by this precedence chain:

1. `viewOverride != ""` → use `viewOverride`
2. `cfg.EntryPointViewID != ""` → use `cfg.EntryPointViewID`
3. Else → `"home"`

#### Scenario: View override wins over config (RED → GREEN)

- GIVEN a valid config with `EntryPointViewID = "dashboard"`
- WHEN `RunBootstrap` is called with `viewOverride = "settings"`
- THEN the initial screen SHALL be loaded with view ID `"settings"`

#### Scenario: Empty override falls through to config (RED → GREEN)

- GIVEN a valid config with `EntryPointViewID = "transacciones_list"`
- WHEN `RunBootstrap` is called with `viewOverride = ""`
- THEN the initial screen SHALL be loaded with view ID `"transacciones_list"`

#### Scenario: Both empty — defaults to "home" (RED → GREEN)

- GIVEN a valid config with `EntryPointViewID = ""`
- WHEN `RunBootstrap` is called with `viewOverride = ""`
- THEN the initial screen SHALL be loaded with view ID `"home"`

### Requirement: Backward Compatibility

Adding `viewOverride` to `RunBootstrap` MUST NOT change the behavior of existing tests. All six existing test call sites in `cmd/golemui/main_test.go` MUST pass after being updated to supply `""` as the 5th argument.

#### Scenario: Existing tests pass with empty viewOverride (REFACTOR)

- GIVEN all six existing `TestRunBootstrap_*` functions
- WHEN each is updated to pass `""` as 5th argument to `RunBootstrap`
- THEN all tests SHALL pass without behavioral change

## TDD Cycle Order

Scenarios are ordered RED → GREEN → REFACTOR:

1. **RED**: Write `TestRunBootstrap_ViewOverrideWins`, `TestRunBootstrap_EmptyOverrideFallsThrough`, `TestRunBootstrap_BothEmptyDefaultsHome` — all fail (signature doesn't accept viewOverride yet)
2. **GREEN**: Add `viewOverride string` parameter, implement override chain, update all 6 call sites — all 9 tests pass
3. **REFACTOR**: Verify no behavioral regression in existing tests
