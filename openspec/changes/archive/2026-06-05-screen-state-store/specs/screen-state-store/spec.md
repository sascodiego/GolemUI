# Screen State Store Specification

## Purpose

Centralized, thread-safe, per-screen key-value store that aggregates all input values for a single screen. All inputs belonging to the same screen write into the same `ScreenState` instance, and the store provides an atomic snapshot for downstream consumers.

## Requirements

### Requirement: Thread-safe State Map

The system SHALL provide a `ScreenState` type containing a `map[string]any` protected by `sync.RWMutex`. The system MUST support concurrent writes from input goroutines and reads from SUBMIT handler goroutines without data races.

#### Scenario: Multi-input convergence

- GIVEN a `ScreenState` instance with keys `"title"` and `"amount"` not yet set
- WHEN two separate goroutines call `state.Set("title", "Go")` and `state.Set("amount", 42)`
- THEN `state.Snapshot()` MUST return `{"title": "Go", "amount": 42}`
- AND both key-value pairs MUST be present in the snapshot

#### Scenario: Overwrite existing key

- GIVEN a `ScreenState` with `state.Set("title", "old")`
- WHEN `state.Set("title", "new")` is called
- THEN `state.Snapshot()["title"]` MUST equal `"new"`

### Requirement: Defensive Snapshot

The `Snapshot()` method MUST return a shallow copy of the internal map. Mutations to the returned map SHALL NOT affect the store's internal state.

#### Scenario: Snapshot isolation

- GIVEN a `ScreenState` with `state.Set("title", "Go")`
- WHEN the caller obtains `m := state.Snapshot()` and sets `m["title"] = "mutated"`
- THEN `state.Snapshot()["title"]` MUST still equal `"Go"`

### Requirement: Input Key Mapping

The key used to store an input value in `ScreenState` SHALL be the value of the input node's `bind_to` metadata property. Inputs without a `bind_to` SHALL NOT write to the store.

#### Scenario: Input writes to bind_to key

- GIVEN a `text_input` node with `bind_to` set to `"filter:title"`
- WHEN the user types `"Golem"` into the input
- THEN the compositor MUST call `state.Set("filter:title", "Golem")`

#### Scenario: Input without bind_to is ignored

- GIVEN a `text_input` node with an empty `bind_to`
- WHEN the user types `"anything"`
- THEN the compositor SHALL NOT call `state.Set` for that input
