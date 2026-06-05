# Technical Design: GolemUI Data Grid Async Loading

This document details the implementation design for introducing asynchronous loading in the GolemUI `data_grid` component.

## Context & Key Decisions

The `data_grid` component rendered using Fyne's `widget.Table` currently blocks the main thread if data fetching is synchronous. To solve this, we decouple data retrieval and UI rendering using a background goroutine and a thread-safe model structure.

### Architectural Decisions

| Decision | Approach | Rationale (Why) |
| :--- | :--- | :--- |
| **Data Ingestion** | Use `db.DatabasePool` injected globally as `ui.BusinessPool` | Enables components in package `ui` to perform business data operations decoupled from `cmd/golemui`. |
| **State Storage** | Define `dataGridModel` representing the table state | Holds mutex and string representations of table cells to ensure thread safety between query and UI rendering threads. |
| **UI Widget** | Use `widget.NewTableWithHeaders` | Provides native sticky column/row headers and cleanest separation of cell and header rendering. |
| **Async Updates** | Spawns background goroutine for pgx Query and updates UI via `fyne.Do()` | Keeps the main UI thread interactive while queries run in the background. |

---

## File Changes & Code Contracts

### 1. `pkg/db/mock_db.go`
Modify the mock row type to supply column metadata matching production.

```go
// In pkg/db/mock_db.go
func (m *MockRows) FieldDescriptions() []pgconn.FieldDescription {
	m.mu.Lock()
	defer m.mu.Unlock()
	desc := make([]pgconn.FieldDescription, len(m.columns))
	for i, col := range m.columns {
		desc[i] = pgconn.FieldDescription{Name: col}
	}
	return desc
}
```

### 2. `pkg/ui/compositor.go`
Introduce the global business pool reference, model structure, and asynchronous query loop.

```go
package ui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"GolemUI/pkg/db"
)

// BusinessPool represents the database pool injected from main.go
var BusinessPool db.DatabasePool

type dataGridModel struct {
	mu      sync.RWMutex
	columns []string
	rows    [][]string
}

func Compose(node NodeMeta) (fyne.CanvasObject, error) {
	// ... inside switch node.ComponentRef
	case "data_grid":
		model := &dataGridModel{}

		table := widget.NewTableWithHeaders(
			func() (int, int) {
				model.mu.RLock()
				defer model.mu.RUnlock()
				return len(model.rows), len(model.columns)
			},
			func() fyne.CanvasObject {
				return widget.NewLabel("")
			},
			func(id widget.TableCellID, o fyne.CanvasObject) {
				model.mu.RLock()
				defer model.mu.RUnlock()
				if id.Row < len(model.rows) && id.Col < len(model.columns) {
					o.(*widget.Label).SetText(model.rows[id.Row][id.Col])
				}
			},
		)

		table.ShowHeaderRow = true
		table.ShowHeaderColumn = true
		table.CreateHeader = func() fyne.CanvasObject {
			return widget.NewLabel("")
		}
		table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
			model.mu.RLock()
			defer model.mu.RUnlock()
			l := o.(*widget.Label)
			if id.Row == -1 && id.Col >= 0 && id.Col < len(model.columns) {
				l.SetText(model.columns[id.Col])
			} else if id.Col == -1 && id.Row >= 0 && id.Row < len(model.rows) {
				l.SetText(strconv.Itoa(id.Row + 1))
			} else {
				l.SetText("")
			}
		}

		if node.DataSource != "" {
			go fetchGridDataAsync(node.DataSource, model, table)
		}
		return table, nil
}

func fetchGridDataAsync(query string, model *dataGridModel, table *widget.Table) {
	if BusinessPool == nil {
		log.Println("Warning: BusinessPool is not initialized")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := BusinessPool.Query(ctx, query)
	if err != nil {
		log.Printf("Error executing data grid query %q: %v", query, err)
		return
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	var cols []string
	for _, fd := range fieldDescriptions {
		cols = append(cols, fd.Name)
	}

	var data [][]string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			log.Printf("Error scanning data grid row values: %v", err)
			return
		}
		rowStr := make([]string, len(vals))
		for i, val := range vals {
			if val == nil {
				rowStr[i] = ""
			} else {
				rowStr[i] = fmt.Sprintf("%v", val)
			}
		}
		data = append(data, rowStr)
	}

	model.mu.Lock()
	model.columns = cols
	model.rows = data
	model.mu.Unlock()

	fyne.Do(func() {
		table.Refresh()
	})
}
```

### 3. `cmd/golemui/main.go`
Assign the initialized business pool during bootstrap.

```diff
// cmd/golemui/main.go
 	dbPool, err := initDB(ctx, coreCfg, bizCfg)
 	if err != nil {
 		return nil, fmt.Errorf("failed to initialize database: %w", err)
 	}
+
+	// Inject the business database pool to UI package
+	ui.BusinessPool = dbPool.BusinessPool
```

---

## Testing Strategy

To safely verify asynchronous rendering without graphical environment requirements, we write programmatic tests in `pkg/ui/compositor_test.go` using `MockDBPool`.

### Test Steps
1. Instantiate `db.NewMockDBPool()` and register a query returned from a stub with mock columns and rows.
2. Inject the mock into `ui.BusinessPool`.
3. Call `Compose()` with a node of type `"data_grid"`.
4. Run a polling loop with `time.Sleep` and a 1-second timeout checking `table.Length()`.
5. Once loaded, assert:
   - Row and column lengths match registered stubs.
   - Cells update correctly via `table.UpdateCell()`.
   - Headers populate correctly via `table.UpdateHeader()`.
