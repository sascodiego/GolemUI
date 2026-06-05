# Delta for Composite Layout Engine (composite-layout-engine)

## MODIFIED Requirements

### Requirement: Compose receives and threads ScreenState

The `Compose` public function MUST create a new `ScreenState` instance and pass it to an internal recursive `composeWithState` function. The recursive function receives `*ScreenState` as a parameter and threads it to all child nodes. All widget factories within `composeWithState` (text_input, button, data_grid) MUST use this state instance for reading and writing.

(Previously: `Compose` was a simple recursive function with no shared state parameter.)

#### Scenario: Successful recursive layout with state threading

- GIVEN a valid `composite_screen` JSON schema with a container holding a `text_input` and a `data_grid`
- WHEN `Compose` is called on the root node
- THEN a single `ScreenState` instance MUST be created for the screen
- AND the `text_input` factory MUST write to that state on user input
- AND the `data_grid` factory MUST subscribe using that same state's snapshot

#### Scenario: Error handling for malformed nodes unchanged

- GIVEN a `composite_screen` JSON payload containing a node with an unrecognized component type
- WHEN the layout engine attempts to parse and render the tree
- THEN the engine MUST log a validation warning and render a fallback placeholder
- AND the `ScreenState` SHALL NOT be affected by the malformed node

#### Scenario: Custom fractional grid metric evaluation unchanged

- GIVEN a grid container node specifying columns `"3fr, 1fr"`
- WHEN the engine renders the screen
- THEN 75% width is allocated to the first column and 25% to the second
- AND `ScreenState` threading is unaffected by layout metrics
