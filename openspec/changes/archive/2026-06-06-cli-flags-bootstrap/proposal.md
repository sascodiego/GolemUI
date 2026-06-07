# Proposal: CLI Flags for GolemUI Bootstrap

## Intent

Enable dynamic CLI configuration of the GolemUI binary entry point: config file path (`-config`) and initial view override (`-view`). Today both values are hardcoded (`"golemui_driver.lua"` and `cfg.EntryPointViewID`) — operators cannot change them without modifying source.

## Scope

### In Scope
- Add `flag.String` vars for `-config` (default `"golemui_driver.lua"`) and `-view` (default `""`) in `cmd/golemui/main.go`
- Call `flag.Parse()` in `main()` before `RunBootstrap`
- Add `viewOverride string` as 5th parameter to `RunBootstrap`
- Implement override chain: `viewOverride != ""` → wins; `""` → `cfg.EntryPointViewID` → `"home"`
- Update 6 existing test call sites to pass `""` as 5th arg (backward compat)
- Add 3 new TDD test cases: override-wins, empty-override-falls-through, both-empty-defaults-home

### Out of Scope
- Changes to `BootstrapConfig` struct or `LoadConfig()`
- Changes to `sanitizeLocale()` or env var logic
- Third-party CLI libraries (cobra, kong, etc.)
- Environment variable flag fallbacks

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `client-bootstrap`: Adding CLI flag parsing and view override resolution to the bootstrap sequence. Requirement change: bootstrap MUST accept a `viewOverride` parameter and resolve the initial view via a three-level override chain.

## Approach

1. Declare package-level `flag.String` vars for `-config` and `-view` with defaults
2. `flag.Parse()` in `main()` only — `RunBootstrap` stays CLI-agnostic
3. Add `viewOverride string` as 5th param to `RunBootstrap`
4. Replace lines 108-111 in `RunBootstrap` with override chain: `viewOverride` → `cfg.EntryPointViewID` → `"home"`
5. Update all 6 existing tests to pass `""` (preserving current behavior)
6. Write 3 new tests (TDD RED first)

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/golemui/main.go` | Modified | Add flag vars, `flag.Parse()`, `viewOverride` param, override chain |
| `cmd/golemui/main_test.go` | Modified | Update 6 call sites + add 3 new test cases |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Param list grows to 5 | Low | Acceptable for this scope; options pattern deferred |
| All 6 tests break if signature change is partial | Med | Atomic commit: signature + all call sites together |

## Rollback Plan

Remove the 5th parameter from `RunBootstrap`, restore hardcoded `"golemui_driver.lua"` in `main()`, revert tests to 4-arg calls. Single commit revert.

## Dependencies

- None (stdlib `flag` package only)

## Success Criteria

- [ ] `-config` with nonexistent file → bootstrap returns file-not-found error
- [ ] `-view` flag → `RunBootstrap` uses that view ID, ignoring `cfg.EntryPointViewID`
- [ ] No args → loads `"golemui_driver.lua"` with configured view (backward compat)
- [ ] All existing + new tests pass
