# Technical Design: GolemUI Go Client Base Structure

This document outlines the architectural specifications, file layouts, interface definitions, and concurrency patterns for the GolemUI Go desktop client.

## 1. File Layout

The client base structure conforms to Clean Architecture and standard Go project layouts:

- **`cmd/golemui/main.go`**: Program entrypoint. Handles initialization sequences, calling `pkg/lua` and `pkg/db`.
- **`pkg/db/db.go`**: Separate Pgx connection pools for `golemui_core` and `negocio_production`.
- **`pkg/lua/loader.go`**: Ephemeral Gopher-Lua VM parser for configurations.
- **`pkg/eventbus/eventbus.go`**: Decoupled thread-safe event broker.
- **`pkg/ui/compositor.go`**: Recursive Fyne compositor rendering `NodeMeta` component trees.
- **`pkg/ui/layout.go`**: Custom grid layout calculating fractional dimensions (`fr`, `auto`).

---

## 2. Key Types & Interfaces

### NodeMeta & Layout JSON Schema Representation
```go
package ui

type LayoutMeta struct {
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
	Rows    []string `json:"rows"`
	Gap     string   `json:"gap"`
}

type NodeMeta struct {
	Area         string     `json:"area"`
	ComponentRef string     `json:"component_ref"`
	Label        string     `json:"label,omitempty"`
	Placeholder  string     `json:"placeholder,omitempty"`
	DefaultValue string     `json:"default_value,omitempty"`
	Min          float64    `json:"min,omitempty"`
	Max          float64    `json:"max,omitempty"`
	Validation   string     `json:"validation,omitempty"`
	DataSource   string     `json:"data_source,omitempty"`
	SubmitAction string     `json:"submit_action,omitempty"`
	BindTo       string     `json:"bind_to,omitempty"`
	Layout       LayoutMeta `json:"layout,omitempty"`
	Children     []NodeMeta `json:"children,omitempty"`
}
```

### Decoupled Reactive EventBus
```go
package eventbus

type Event struct {
	Channel string
	Payload interface{}
}

type Handler func(Event)

type EventBus interface {
	Publish(channel string, payload interface{})
	Subscribe(channel string, h Handler) string // Returns unique sub ID
	Unsubscribe(channel string, subID string)
}
```

### Ephemeral Lua Bootstrapper Connection Configs
```go
package lua

type ConfigConexion struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type BootstrapConfig struct {
	UIDB            ConfigConexion
	BusinessDB      ConfigConexion
	EntryPointQuery string
}
```

### Fractional Layout Engine
```go
package ui

import "fyne.io/fyne/v2"

type FractionalLayout struct {
	Columns []string
	Rows    []string
	Gap     float32
}

// Implements fyne.Layout interface
func (l *FractionalLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {}
func (l *FractionalLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(100, 100)
}
```

---

## 3. Architecture Decisions

| Component | Choice | Rationale |
| :--- | :--- | :--- |
| **Project Layout** | Standard `/pkg` structure | Isolates domain logic (`db`, `lua`, `eventbus`, `ui`), enforcing clear boundary imports. |
| **EventBus Flow** | Interface-based subscription callback | `pkg/eventbus` depends on primitive types only, preventing circular imports with UI. |
| **Lua Bootstrapper** | Ephemeral `defer L.Close()` VM | Minimizes memory leaks; loads initial settings into Go memory then immediately frees runtime resources. |
| **Database Pool** | Segregated Pgx Connection Pools | Maintains strict boundary segregation between core logic metadata and transaction payloads. |

### Circular Imports Risk & Mitigation
UI components must publish/subscribe via `eventbus.EventBus`. To completely avoid import cycles:
1. `pkg/eventbus` holds no references to the UI or compositor packages.
2. `pkg/ui` imports `pkg/eventbus` and uses the callback interface `Handler` to handle events.

### Lua VM Cleanup
`pkg/lua/loader.go` must wrap the execution of the state engine inside a cleanup scope:
```go
func LoadConfig(path string) (*BootstrapConfig, error) {
	L := lua.NewState()
	defer L.Close() // Safe memory release
	// Exec and extract table variables...
}
```

---

## 4. UI Concurrency Patterns

Fyne requires visual mutations to happen on the main UI thread. For background tasks (e.g., retrieving database rows):
1. **Background Fetch**: Spawns a goroutine to fetch data via `negocio_production` connection.
2. **Main Thread Callback**: Dispatches UI updates through Fyne's queue helper.
```go
// Concurrency template
go func() {
	data, err := db.FetchData(...)
	if err != nil {
		return
	}
	// Safely queue UI mutation on main thread
	fyne.CurrentApp().Driver().CanvasForObject(table).InteractiveArea().QueueEvent(func() {
		widgetData.Update(data)
		table.Refresh()
	})
}()
```

---

## 5. Testing Strategy

1. **Unit Testing**:
   - `pkg/eventbus`: Verify concurrency safety under 50 simultaneous readers/writers.
   - `pkg/lua`: Test config parsing with mock config files.
   - `pkg/ui`: Assert container calculations in `FractionalLayout.MinSize`.
2. **Integration Testing**:
   - Verify connection pool initialization and health checks against Docker Postgres.
