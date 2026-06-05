package ui

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
)

var BusinessPool db.DatabasePool
var CorePool     db.DatabasePool
var LocalEventBus eventbus.EventBus

type dataGridModel struct {
	mu            sync.RWMutex
	headers       []string
	columns       []string
	rows          [][]string
	masterHeaders []string
	masterRows    [][]string
	filterKeys    []string
	cancel        context.CancelFunc
	unsubscribe   func()
}

type LayoutMeta struct {
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
	Rows    []string `json:"rows"`
	Gap     string   `json:"gap"`
}

type NodeMeta struct {
	Area             string     `json:"area"`
	ComponentRef     string     `json:"component_ref"`
	Label            string     `json:"label,omitempty"`
	Placeholder      string     `json:"placeholder,omitempty"`
	DefaultValue     string     `json:"default_value,omitempty"`
	Min              float64    `json:"min,omitempty"`
	Max              float64    `json:"max,omitempty"`
	Validation       string     `json:"validation,omitempty"`
	DataSource       string     `json:"data_source,omitempty"`
	SubmitAction     string     `json:"submit_action,omitempty"`
	BindTo           string     `json:"bind_to,omitempty"`
	FilterMode       string     `json:"filter_mode,omitempty"`
	MasterDataSource string     `json:"master_data_source,omitempty"`
	FilterKeys       []string   `json:"filter_keys,omitempty"`
	Layout           LayoutMeta `json:"layout,omitempty"`
	Children         []NodeMeta `json:"children,omitempty"`
}

// Compose creates a per-screen ScreenState scoped to vistaID and delegates to composeWithState.
func Compose(node NodeMeta, vistaID string) (fyne.CanvasObject, error) {
	state := NewScreenState(vistaID)
	return composeWithState(node, state)
}

// composeWithState recursively builds Fyne widgets, threading *ScreenState through all children.
func composeWithState(node NodeMeta, state *ScreenState) (fyne.CanvasObject, error) {
	switch node.ComponentRef {
	case "container":
		var objects []fyne.CanvasObject
		for _, child := range node.Children {
			cObj, err := composeWithState(child, state)
			if err != nil {
				return nil, err
			}
			objects = append(objects, cObj)
		}

		switch node.Layout.Type {
		case "vertical":
			return container.NewVBox(objects...), nil
		case "horizontal":
			return container.NewHBox(objects...), nil
		case "grid":
			var gap float64
			if node.Layout.Gap != "" {
				if g, err := strconv.ParseFloat(node.Layout.Gap, 32); err == nil {
					gap = g
				}
			}
			lay := &FractionalLayout{
				Columns: node.Layout.Columns,
				Rows:    node.Layout.Rows,
				Gap:     float32(gap),
			}
			return container.New(lay, objects...), nil
		default:
			return container.NewHBox(objects...), nil
		}

	case "label":
		return widget.NewLabel(node.Label), nil

	case "text_input":
		entry := widget.NewEntry()
		entry.PlaceHolder = node.Placeholder
		entry.SetText(node.DefaultValue)
		if node.BindTo != "" {
			entry.OnChanged = func(text string) {
				state.Set(node.BindTo, text)
			}
		}
		return entry, nil

	case "button":
		if node.SubmitAction != "" && LocalEventBus != nil {
			return widget.NewButton(node.Label, func() {
				LocalEventBus.Publish(state.SubmitChannel(), state.Snapshot())
			}), nil
		}
		return widget.NewButton(node.Label, func() {}), nil

	case "data_grid":
		model := &dataGridModel{
			filterKeys: node.FilterKeys,
		}
		table := widget.NewTableWithHeaders(
			func() (int, int) {
				model.mu.RLock()
				defer model.mu.RUnlock()
				return len(model.rows), len(model.headers)
			},
			func() fyne.CanvasObject {
				return widget.NewLabel("")
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				model.mu.RLock()
				defer model.mu.RUnlock()
				if id.Row < 0 || id.Row >= len(model.rows) || id.Col < 0 || id.Col >= len(model.headers) {
					return
				}
				row := model.rows[id.Row]
				if id.Col < len(row) {
					if label, ok := cell.(*widget.Label); ok {
						label.SetText(row[id.Col])
					}
				}
			},
		)

		table.CreateHeader = func() fyne.CanvasObject {
			return widget.NewLabel("")
		}

		table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
			model.mu.RLock()
			defer model.mu.RUnlock()
			if id.Col >= 0 && id.Col < len(model.headers) {
				if label, ok := cell.(*widget.Label); ok {
					label.SetText(model.headers[id.Col])
				}
			}
		}

		model.mu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		model.cancel = cancel
		model.mu.Unlock()

		// Client-mode: eagerly load master buffer
		if node.FilterMode == "client" && node.MasterDataSource != "" {
			loadMasterBuffer(ctx, node, model, table)
		} else if node.DataSource != "" {
			// Default / server-mode: load initial data using initial state parameters
			args := extractOrderedArgs(state.Snapshot(), node.FilterKeys)
			fetchGridDataAsync(ctx, node, model, table, args...)
		}

		// Subscribe to scoped SubmitChannel for reactivity
		if LocalEventBus != nil {
			subID := LocalEventBus.Subscribe(state.SubmitChannel(), func(ev eventbus.Event) {
				snap, ok := ev.Payload.(map[string]any)
				if !ok {
					return
				}

				if node.FilterMode == "client" {
					// Client-side filtering: filter masterRows in memory
					filterMasterRows(model, table, snap)
				} else {
					// Server-side filtering: parameterized query
					if len(node.FilterKeys) == 0 {
						log.Printf("Warning: server-mode data_grid at area %q requires filter_keys but none defined; skipping SUBMIT", node.Area)
						return
					}

					model.mu.Lock()
					if model.cancel != nil {
						model.cancel()
					}
					subCtx, subCancel := context.WithCancel(context.Background())
					model.cancel = subCancel
					model.mu.Unlock()

					args := extractOrderedArgs(snap, node.FilterKeys)
					fetchGridDataAsync(subCtx, node, model, table, args...)
				}
			})
			model.mu.Lock()
			model.unsubscribe = func() {
				LocalEventBus.Unsubscribe(state.SubmitChannel(), subID)
			}
			model.mu.Unlock()
		}

		return table, nil

	default:
		log.Printf("Warning: Unrecognized component type %q at area %q", node.ComponentRef, node.Area)
		fallback := widget.NewLabel(fmt.Sprintf("[Fallback: Unrecognized component type %q]", node.ComponentRef))
		return fallback, nil
	}
}

// extractOrderedArgs maps snapshot keys to positional args ($1, $2, ...) in FilterKeys order.
// Missing keys default to empty string (so LIKE '' matches everything instead of NULL = false).
// Returns empty slice when filterKeys is empty — no alphabetical fallback.
func extractOrderedArgs(snap map[string]any, filterKeys []string) []any {
	if len(filterKeys) == 0 {
		return []any{}
	}
	args := make([]any, 0, len(filterKeys))
	for _, key := range filterKeys {
		val, exists := snap[key]
		if !exists {
			args = append(args, "")
		} else {
			args = append(args, val)
		}
	}
	return args
}

// loadMasterBuffer eagerly loads all data from MasterDataSource into the model's masterRows.
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
	if BusinessPool == nil {
		log.Printf("Warning: BusinessPool is nil; cannot load master buffer for data_grid at area %q", node.Area)
		return
	}
	go func() {
		if err := ctx.Err(); err != nil {
			return
		}
		rows, err := BusinessPool.Query(ctx, node.MasterDataSource)
		if err != nil {
			log.Printf("Error loading master buffer %q: %v", node.MasterDataSource, err)
			return
		}
		defer rows.Close()

		fds := rows.FieldDescriptions()
		var headers []string
		for _, fd := range fds {
			headers = append(headers, fd.Name)
		}

		var dataRows [][]string
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return
			}
			vals, err := rows.Values()
			if err != nil {
				log.Printf("Error scanning master row values: %v", err)
				break
			}
			var stringRow []string
			for _, val := range vals {
				if val == nil {
					stringRow = append(stringRow, "")
				} else {
					stringRow = append(stringRow, fmt.Sprintf("%v", val))
				}
			}
			dataRows = append(dataRows, stringRow)
		}

		if err := ctx.Err(); err != nil {
			return
		}

		model.mu.Lock()
		model.masterHeaders = headers
		model.masterRows = dataRows
		model.headers = headers
		model.rows = dataRows
		model.mu.Unlock()

		fyne.Do(func() {
			table.Refresh()
		})
	}()
}

// filterMasterRows applies client-side filtering on the master buffer.
// A row matches if ALL snapshot values appear as substrings in the corresponding columns.
func filterMasterRows(model *dataGridModel, table *widget.Table, snap map[string]any) {
	model.mu.Lock()

	if len(model.masterRows) == 0 {
		model.mu.Unlock()
		return
	}

	// Build column index from master headers
	colIndex := make(map[string]int)
	for i, h := range model.masterHeaders {
		colIndex[h] = i
	}

	// If snapshot is empty, show all rows
	if len(snap) == 0 {
		model.rows = model.masterRows
		model.mu.Unlock()
		fyne.Do(func() {
			table.Refresh()
		})
		return
	}

	var filtered [][]string
	for _, row := range model.masterRows {
		match := true
		for key, val := range snap {
			col, ok := colIndex[key]
			if !ok {
				log.Printf("Warning: client-mode filter key %q not found in grid columns %v; skipping", key, model.masterHeaders)
				continue // key not in grid columns — skip
			}
			if col >= len(row) {
				match = false
				break
			}
			cellVal := row[col]
			searchStr := fmt.Sprintf("%v", val)
			if searchStr == "" {
				continue // empty filter matches all
			}
			// Substring match
			if !containsIgnoreCase(cellVal, searchStr) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, row)
		}
	}

	model.rows = filtered
	model.mu.Unlock()

	fyne.Do(func() {
		table.Refresh()
	})
}

// containsIgnoreCase checks if substr is contained in s, case-insensitive.
func containsIgnoreCase(s, substr string) bool {
	sl := len(s)
	subl := len(substr)
	if subl == 0 {
		return true
	}
	if subl > sl {
		return false
	}
	for i := 0; i <= sl-subl; i++ {
		if caseInsensitiveEqual(s[i:i+subl], substr) {
			return true
		}
	}
	return false
}

func caseInsensitiveEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca := a[i]
		cb := b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func fetchGridDataAsync(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table, args ...any) {
	if node.DataSource == "" {
		return
	}
	go func() {
		if BusinessPool == nil {
			log.Printf("Warning: BusinessPool is nil; cannot execute query for data_grid at area %q", node.Area)
			return
		}
		if err := ctx.Err(); err != nil {
			return
		}
		rows, err := BusinessPool.Query(ctx, node.DataSource, args...)
		if err != nil {
			log.Printf("Error running data_grid query %q: %v", node.DataSource, err)
			return
		}
		defer rows.Close()

		fds := rows.FieldDescriptions()
		var headers []string
		for _, fd := range fds {
			headers = append(headers, fd.Name)
		}

		var dataRows [][]string
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return
			}
			vals, err := rows.Values()
			if err != nil {
				log.Printf("Error scanning row values: %v", err)
				break
			}
			var stringRow []string
			for _, val := range vals {
				if val == nil {
					stringRow = append(stringRow, "")
				} else {
					stringRow = append(stringRow, fmt.Sprintf("%v", val))
				}
			}
			dataRows = append(dataRows, stringRow)
		}

		if err := ctx.Err(); err != nil {
			return
		}

		model.mu.Lock()
		model.headers = headers
		model.columns = headers
		model.rows = dataRows
		model.mu.Unlock()

		fyne.Do(func() {
			table.Refresh()
		})
	}()
}
