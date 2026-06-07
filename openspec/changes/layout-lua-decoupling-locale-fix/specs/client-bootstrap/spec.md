# Delta for client-bootstrap

## ADDED Requirements

### Requirement: LayoutQuery Flow Through Bootstrap

When `RunBootstrap` executes, it SHALL pass `cfg.LayoutQuery` to every `ui.LoadScreen` call site — both the initial home screen load and the `ui.Navigate` closure used for in-app navigation.

#### Scenario: LayoutQuery flows to initial LoadScreen call

- GIVEN a `golemui_driver.lua` with `LayoutQuery = "SELECT custom_col FROM views WHERE id = $1"`
- WHEN `RunBootstrap` reaches the initial `LoadScreen` call
- THEN `cfg.LayoutQuery` SHALL be passed as the `layoutQuery` argument

#### Scenario: LayoutQuery flows to Navigate closure

- GIVEN a `golemui_driver.lua` with `LayoutQuery = "SELECT custom_col FROM views WHERE id = $1"`
- AND the `ui.Navigate` callback is invoked with any vista ID
- WHEN `LoadScreen` is called inside the closure
- THEN the captured `cfg.LayoutQuery` SHALL be passed as the `layoutQuery` argument

#### Scenario: Empty LayoutQuery works via fallback

- GIVEN a `golemui_driver.lua` without a `LayoutQuery` key
- WHEN `RunBootstrap` executes both `LoadScreen` call sites
- THEN an empty string SHALL be passed as `layoutQuery`
- AND `LoadScreen` SHALL fall back to `DefaultLayoutQuery` without error
