# Specification: Reactive Input Publishing (reactive-input-publishing)

## Introduction
The `reactive-input-publishing` capability specifies how user input events in GolemUI text fields (`widget.Entry` or similar) are captured and published. When a user modifies the text, the UI compositor intercepts the changes and broadcasts the new state over a local EventBus under a specific channel topic defined by the component's `BindTo` metadata. This allows downstream reactive widgets, such as data grids, to receive updates and adapt their display.

## Requirements
1. The GolemUI client compositor MUST inspect the `bind_to` metadata property of `text_input` and `button` components.
2. If the `bind_to` property is not empty, the compositor MUST register a listener callback on the input widget's state change event (`OnChanged` for `widget.Entry` or click handlers for buttons).
3. The widget's callback handler MUST publish the updated input value (or click event payload) to the local event bus under the channel named by `bind_to`.
4. If the `bind_to` property is empty or absent, the compositor SHALL NOT register any event publishing callback for that widget.
5. The callback handler MUST publish the payload without blocking the UI thread.
6. The event payload MUST contain the updated string value of the input or the action trigger payload.

## Scenarios

### Scenario 1: State Publishing on Text Input Change
*   **GIVEN** a text input widget is rendered with `bind_to` set to `"filter:title"`
*   **WHEN** the user types the text `"Go"` into the input widget
*   **THEN** the input widget's `OnChanged` handler MUST publish the value `"Go"` to the `"filter:title"` event channel
*   **AND** the event MUST be received by all active subscribers of `"filter:title"`.

### Scenario 2: No Publishing when BindTo is Empty
*   **GIVEN** a text input widget is rendered with an empty `bind_to` metadata property
*   **WHEN** the user types the text `"Fyne"` into the input widget
*   **THEN** the compositor SHALL NOT publish any events to the event bus
*   **AND** no pub/sub overhead is incurred.

### Scenario 3: Event Trigger on Button Click
*   **GIVEN** a button widget is rendered with `bind_to` set to `"search:trigger"`
*   **WHEN** the user clicks the button
*   **THEN** the button's tapped handler MUST publish an action payload to the `"search:trigger"` event channel.
