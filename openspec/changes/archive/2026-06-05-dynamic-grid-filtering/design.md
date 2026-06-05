# Design: Dynamic Grid Filtering

## 1. Overview
This design implements dynamic filtering in GolemUI. Text input components publish change events to a shared event bus, which data grids subscribe to for running parameterized queries. Concurrency control via context cancellation prevents out-of-order UI updates when typing rapidly.

---

## 2. Key Decisions

| Decision | Choice | Rationale |
|---|---|---|
| **Event Bus Wiring** | Assign `ui.LocalEventBus = eb` in `main.go` bootstrap | Exposes the event bus package-wide for loose coupling between input and grid widgets. |
| **Input Value Publish** | Bind to `entry.OnChanged` on `"text_input"` | Captures user keystrokes dynamically without requiring a submit button. |
| **Parameterized Querying** | Pass event payload directly as `args ...any` to `BusinessPool.Query` | Leverages native PostgreSQL parameter binding (`$1`) for SQL injection security. |
| **Concurrency Control** | Keep `context.CancelFunc` on `dataGridModel`, cancelling before executing new query | Ensures stale, slow queries are aborted so they don't overwrite newer results. |
| **Subscription Cleanup** | Store `unsubscribe func()` on `dataGridModel` | Allows clean memory management and prevents event bus leaks. |

---

## 3. Contracts and Interfaces

```go
package ui

import (
	"context"
	"sync"
	"fyne.io/fyne/v2/widget"
	"GolemUI/pkg/eventbus"
)

// LocalEventBus is configured during main.go bootstrap
var LocalEventBus eventbus.EventBus

// dataGridModel holds rows metadata, database parameters, and context management
type dataGridModel struct {
	mu          sync.RWMutex
	headers     []string
	columns     []string
	rows        [][]string
	cancel      context.CancelFunc // active query context cancellation function
	unsubscribe func()             // cleans up eventbus subscription
}

// fetchGridDataAsync executes the query asynchronously and refreshes the table
func fetchGridDataAsync(ctx context.Context, query string, model *dataGridModel, table *widget.Table, args ...any)
```

---

## 4. Concrete Changes

### [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go)
- After instantiating `eb := eventbus.NewEventBus()`, assign:
  ```go
  ui.LocalEventBus = eb
  ```

### [pkg/ui/compositor.go](file:///src/GolemUI/pkg/ui/compositor.go)
- Import `context` and `"GolemUI/pkg/eventbus"`.
- Declare global variable `var LocalEventBus eventbus.EventBus`.
- Add `cancel context.CancelFunc` and `unsubscribe func()` fields to the `dataGridModel` struct.
- In `Compose` case `"text_input"`:
  ```go
  if node.BindTo != "" {
      entry.OnChanged = func(val string) {
          if LocalEventBus != nil {
              LocalEventBus.Publish(node.BindTo, val)
          }
      }
  }
  ```
- In `Compose` case `"data_grid"`:
  - Create initial cancellable context:
    ```go
    ctx, cancel := context.WithCancel(context.Background())
    model.mu.Lock()
    model.cancel = cancel
    model.mu.Unlock()
    go fetchGridDataAsync(ctx, node.DataSource, model, table)
    ```
  - If `node.BindTo != ""` and `LocalEventBus != nil`, subscribe to the channel:
    ```go
    subID := LocalEventBus.Subscribe(node.BindTo, func(ev eventbus.Event) {
        model.mu.Lock()
        if model.cancel != nil {
            model.cancel()
        }
        newCtx, newCancel := context.WithCancel(context.Background())
        model.cancel = newCancel
        model.mu.Unlock()

        go fetchGridDataAsync(newCtx, node.DataSource, model, table, ev.Payload)
    })
    model.mu.Lock()
    model.unsubscribe = func() {
        LocalEventBus.Unsubscribe(node.BindTo, subID)
    }
    model.mu.Unlock()
    ```
- In `fetchGridDataAsync`:
  - Change signature to:
    ```go
    func fetchGridDataAsync(ctx context.Context, query string, model *dataGridModel, table *widget.Table, args ...any)
    ```
  - Pass `args...` directly: `BusinessPool.Query(ctx, query, args...)`.
  - Check `ctx.Err() != nil` at each step (before query, inside row scanning loop, and before locking model to write rows) to abort if cancelled.

---

## 5. Testing Strategy

We will implement a unit test in `pkg/ui/compositor_test.go` checking the reactive flow:

1. **Mock Database Setup**: Register a parameterized query stub on `MockDBPool` matching:
   - Query: `SELECT * FROM books WHERE title = $1`
   - Parameters: `[]any{"Book A"}`
   - Result: Columns `["id", "title"]`, Rows `[[1, "Book A"]]`
2. **Event Bus Setup**: Instantiate a fresh `eventbus.NewEventBus()` and assign to `ui.LocalEventBus`.
3. **Widget Composition**: Compose a container with:
   - A `"text_input"` bound to `"book_filter"`.
   - A `"data_grid"` bound to `"book_filter"` with `DataSource: "SELECT * FROM books WHERE title = $1"`.
4. **Trigger Events**:
   - Use `test.Type(entry, "Book A")` to trigger entry change events.
5. **Assert Table State**:
   - Loop with `time.Sleep` up to 500ms waiting for the table length to become 1 row and 2 columns.
   - Assert cell contents of row 0 match `"Book A"`.
6. **Assert Query Cancellation**:
   - Verify that sequential calls (e.g. typing `"B"`, then quickly `"Book A"`) trigger cancellation of the first context.
