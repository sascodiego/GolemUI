# Consolidated Submit Specification

## Purpose

A button widget with a non-empty `submit_action` publishes a single SUBMIT event containing the full screen state snapshot. This replaces the old per-keystroke direct publishing model with a user-initiated consolidated event.

## Requirements

### Requirement: Submit Button Emits State Snapshot

When a button node has a non-empty `submit_action`, its `OnTapped` callback MUST publish `state.Snapshot()` to the fixed channel `eventbus.SubmitChannel` (`"screen:submit"`). The payload SHALL be a `map[string]any`.

#### Scenario: Button click publishes consolidated state

- GIVEN a screen with `ScreenState` containing `{("filter:title", "Go"), ("filter:min_amount", 100)}`
- AND a button with `submit_action` set to `"home:submit"`
- WHEN the user clicks the button
- THEN the event bus MUST receive exactly one event on channel `"screen:submit"`
- AND the event payload MUST contain `{"filter:title": "Go", "filter:min_amount": 100}`

#### Scenario: Button without submit_action does nothing

- GIVEN a button with an empty `submit_action`
- WHEN the user clicks the button
- THEN no event SHALL be published to the event bus

### Requirement: Submit Channel Constant

The system SHALL define `eventbus.SubmitChannel` as a package-level constant with value `"screen:submit"`. All submit buttons and all grid subscribers MUST reference this constant, not string literals.

#### Scenario: Constant matches expected value

- GIVEN the `eventbus` package is imported
- WHEN `eventbus.SubmitChannel` is evaluated
- THEN it MUST equal `"screen:submit"`

### Requirement: Snapshot Completeness

The snapshot published in the SUBMIT event MUST reflect all input values written to the store up to the moment of the button click. Partial or stale snapshots are not acceptable.

#### Scenario: Rapid input then submit

- GIVEN a `ScreenState` with two inputs
- WHEN the user types `"Alpha"` in input A, then `"Beta"` in input B, then immediately clicks the submit button
- THEN the SUBMIT payload MUST contain both `"Alpha"` and `"Beta"` values
