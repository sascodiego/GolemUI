# Delta for Reactive Input Publishing (reactive-input-publishing)

## MODIFIED Requirements

### Requirement: Input writes to ScreenState instead of publishing directly

The GolemUI client compositor MUST inspect the `bind_to` metadata property of `text_input` components. If `bind_to` is not empty, the compositor MUST register a listener on `OnChanged` that writes the value into the per-screen `ScreenState` store under the key named by `bind_to`. The callback SHALL NOT publish directly to the event bus. If `bind_to` is empty or absent, the compositor SHALL NOT register any callback for that widget.

(Previously: `text_input` published its value directly to the event bus under the channel named by `bind_to`.)

#### Scenario: State write on text input change

- GIVEN a text input widget with `bind_to` set to `"filter:title"` and a `ScreenState` instance
- WHEN the user types `"Go"` into the input widget
- THEN the compositor MUST call `state.Set("filter:title", "Go")`
- AND no event SHALL be published directly to the event bus

#### Scenario: No write when bind_to is empty

- GIVEN a text input widget with an empty `bind_to` metadata property
- WHEN the user types `"Fyne"` into the input widget
- THEN the compositor SHALL NOT call `state.Set`
- AND no event SHALL be published to the event bus

### Requirement: Button with submit_action publishes SUBMIT

A button with a non-empty `submit_action` MUST publish a consolidated SUBMIT event containing `state.Snapshot()` to `eventbus.SubmitChannel` when clicked. Buttons with empty `submit_action` SHALL NOT publish any event.

(Previously: button `OnTapped` was an empty callback with no event publishing behavior.)

#### Scenario: Button click triggers SUBMIT

- GIVEN a button widget with `submit_action` set to `"home:submit"` and a `ScreenState` containing `{"filter:title": "Go"}`
- WHEN the user clicks the button
- THEN the compositor MUST publish `state.Snapshot()` to `eventbus.SubmitChannel`
- AND the payload MUST be `{"filter:title": "Go"}`

## REMOVED Requirements

### Requirement: Direct event publishing on text input change

(Reason: Replaced by state store write. Inputs no longer publish to the event bus — the submit button consolidates and publishes on behalf of all inputs.)
