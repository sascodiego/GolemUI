## Exploration: dynamic-grid-filtering

### Current State
Currently, `pkg/ui/compositor.go` handles layout composition and component rendering using Fyne. The dynamic components like `text_input` and `data_grid` operate in isolation:
- `text_input` is composed as a simple `widget.Entry` without any event bindings or state-publishing capabilities.
- `button` is composed as a simple `widget.Button` with an empty tapped function callback.
- `data_grid` is composed as a `widget.Table` which asynchronously executes a static SQL query from its `DataSource` using the global `BusinessPool` and refreshes thread-safely via `fyne.Do()`.
- An in-memory event bus is implemented in `pkg/eventbus/eventbus.go` as `InMemEventBus`, but it is not integrated into `pkg/ui/compositor.go`.

### Affected Areas
- **`pkg/ui/compositor.go`**:
  - Introduce `var LocalEventBus eventbus.EventBus` to expose the event bus globally within the package.
  - Import `"GolemUI/pkg/eventbus"`.
  - Modify `Compose` case `"text_input"` to publish changes on `entry.OnChanged` to `LocalEventBus` under the channel defined by `node.BindTo` if not empty.
  - Modify `Compose` case `"button"` to publish click events to `LocalEventBus` under the channel defined by `node.BindTo` when tapped if not empty.
  - Modify `Compose` case `"data_grid"` to subscribe to the channel defined by `node.BindTo` if not empty.
  - Refactor `fetchGridDataAsync` signature to accept optional filter arguments (`args ...any`) to parameterize queries reactively. On event broadcasts, trigger `fetchGridDataAsync` with the event payload to re-run the query and update the table.
- **`cmd/golemui/main.go`**:
  - Wire the initialized EventBus to `ui.LocalEventBus` during bootstrap.
- **`pkg/ui/compositor_test.go`**:
  - Add unit/integration tests that register query stubs on `MockDBPool`, set up `LocalEventBus`, trigger widget events, and assert that the Fyne table model updates thread-safely with filtered query results.

### Approaches

#### Approach 1: Generic Parameter Binding to BusinessPool.Query
Pass the published value directly to the query execution parameter list.
- **Pros**: Matches `DBQuerier.Query` method signature exactly; uses PostgreSQL parameters safely (e.g. `$1`) preventing SQL injection; very low complexity.
- **Cons**: Assumes the query expects exactly one parameter (the text input's value). If the query needs multiple parameters, this requires a more complex binding map or query parser.
- **Effort**: Low (~3-4 hours)

#### Approach 2: String Interpolation/Replacement (Templated Querying)
Parse the search term and interpolate/replace special placeholders (e.g., `{{filter}}`) in `node.DataSource` before passing to `BusinessPool.Query`.
- **Pros**: Highly flexible; allows placing the filter term anywhere inside the query string.
- **Cons**: Susceptible to SQL injection if not properly sanitized; parsing/templating logic increases codebase complexity.
- **Effort**: Medium (~6-8 hours)

### Recommendation
We recommend **Approach 1 (Generic Parameter Binding)**. It is secure, leverages the native query parameter support (`$1`), and aligns with standard database interaction patterns. By simply extending `fetchGridDataAsync` to accept `args ...any` and passing the payload to `BusinessPool.Query(ctx, node.DataSource, args...)`, we can support parametrized filtering safely and cleanly.

### Risks
- **Race Conditions**: Parallel query executions can cause out-of-order table updates if a slower, earlier query completes after a faster, later query. We can mitigate this by utilizing a request counter or context cancellation for stale queries.
- **Mock DB Query Matching**: `MockDBPool` matches query strings exactly. The test query stub must precisely match the query string used by the compositor, including parameter placeholders (e.g., `SELECT * FROM books WHERE title = $1`).

### Ready for Proposal
Yes
