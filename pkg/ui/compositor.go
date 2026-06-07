package ui

import (
	"context"
	"database/sql/driver"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var BusinessPool db.DatabasePool
var CorePool db.DatabasePool
var LocalEventBus eventbus.EventBus
var Navigate func(vistaID string)

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

	case "text_area":
		entry := widget.NewMultiLineEntry()
		entry.PlaceHolder = node.Placeholder
		entry.SetText(node.DefaultValue)
		if node.BindTo != "" {
			entry.OnChanged = func(text string) {
				state.Set(node.BindTo, text)
			}
		}
		return entry, nil

	case "button":
		if strings.HasPrefix(node.SubmitAction, "navigate:") && Navigate != nil {
			targetVista := strings.TrimPrefix(node.SubmitAction, "navigate:")
			return widget.NewButton(node.Label, func() {
				Navigate(targetVista)
			}), nil
		}
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
				lbl := widget.NewLabel("")
				lbl.Truncation = fyne.TextTruncateClip
				return lbl
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				model.mu.RLock()
				defer model.mu.RUnlock()
				label, ok := cell.(*widget.Label)
				if !ok {
					return
				}
				if id.Row < 0 || id.Row >= len(model.rows) || id.Col < 0 || id.Col >= len(model.headers) {
					label.SetText("")
					return
				}
				row := model.rows[id.Row]
				if id.Col < len(row) {
					label.SetText(row[id.Col])
				} else {
					label.SetText("")
				}
			},
		)

		table.CreateHeader = func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			lbl.Truncation = fyne.TextTruncateClip
			return lbl
		}

		table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
			model.mu.RLock()
			defer model.mu.RUnlock()
			label, ok := cell.(*widget.Label)
			if !ok {
				return
			}
			if id.Col >= 0 && id.Col < len(model.headers) {
				label.SetText(model.headers[id.Col])
			} else {
				label.SetText("")
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
			if !strings.HasPrefix(node.DataSource, "state:") {
				args := extractOrderedArgs(state.Snapshot(), node.FilterKeys)
				fetchGridDataAsync(ctx, node, model, table, node.DataSource, args...)
			}
		}

		// Subscribe to scoped SubmitChannel for reactivity
		if LocalEventBus != nil {
			log.Printf("[UI/DataGrid] Subscribing data_grid at area %q to channel %q", node.Area, state.SubmitChannel())
			subID := LocalEventBus.Subscribe(state.SubmitChannel(), func(ev eventbus.Event) {
				snap, ok := ev.Payload.(map[string]any)
				if !ok {
					log.Printf("[UI/DataGrid] Warning: payload on channel %q is not map[string]any", state.SubmitChannel())
					return
				}
				log.Printf("[UI/DataGrid] Reacting to submit channel %q. Filter snapshot: %+v", state.SubmitChannel(), snap)

				if node.FilterMode == "client" {
					// Client-side filtering: filter masterRows in memory
					filterMasterRows(model, table, snap)
				} else {
					// Server-side filtering
					query := node.DataSource
					var args []any
					if strings.HasPrefix(query, "state:") {
						stateKey := strings.TrimPrefix(query, "state:")
						qVal, exists := snap[stateKey]
						if !exists || qVal == "" {
							log.Printf("[UI/DataGrid] Dynamic query key %q is empty; skipping query", stateKey)
							return
						}
						query = fmt.Sprintf("%v", qVal)
					} else {
						if len(node.FilterKeys) == 0 {
							log.Printf("[UI/DataGrid] Warning: server-mode data_grid at area %q requires filter_keys but none defined; skipping SUBMIT", node.Area)
							return
						}
						args = extractOrderedArgs(snap, node.FilterKeys)
					}

					model.mu.Lock()
					if model.cancel != nil {
						model.cancel()
					}
					subCtx, subCancel := context.WithCancel(context.Background())
					model.cancel = subCancel
					model.mu.Unlock()

					fetchGridDataAsync(subCtx, node, model, table, query, args...)
				}
			})
			model.mu.Lock()
			model.unsubscribe = func() {
				LocalEventBus.Unsubscribe(state.SubmitChannel(), subID)
			}
			model.mu.Unlock()
		}

		// Row selection capture: publish selected row as header→value map
		table.OnSelected = func(id widget.TableCellID) {
			model.mu.RLock()
			if id.Row < 0 || id.Row >= len(model.rows) {
				model.mu.RUnlock()
				return
			}
			row := model.rows[id.Row]
			headers := model.headers
			rowMap := make(map[string]any, len(headers))
			for i := 0; i < len(headers) && i < len(row); i++ {
				rowMap[headers[i]] = row[i]
			}
			model.mu.RUnlock()
			if LocalEventBus != nil {
				LocalEventBus.Publish("publish_selection", rowMap)
			}
		}

		return table, nil

	default:
		log.Printf("Warning: Unrecognized component type %q at area %q", node.ComponentRef, node.Area)
		fallback := widget.NewLabel(fmt.Sprintf("[Fallback: Unrecognized component type %q]", node.ComponentRef))
		return fallback, nil
	}
}

// extractOrderedArgs maps snapshot keys to positional args ($1, $2, ...) in FilterKeys order.
// Missing keys default to empty string (so LIKE ” matches everything instead of NULL = false).
// Returns empty slice when filterKeys is empty — no alphabetical fallback.
func extractOrderedArgs(snap map[string]any, filterKeys []string) []any {
	log.Printf("[UI/DataGrid] Debug: extractOrderedArgs called with filterKeys: %+v (len: %d)", filterKeys, len(filterKeys))
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
	log.Printf("[UI/DataGrid] Debug: extractOrderedArgs returning args: %+v (len: %d)", args, len(args))
	return args
}

// loadMasterBuffer eagerly loads all data from MasterDataSource into the model's masterRows.
func loadMasterBuffer(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table) {
	if BusinessPool == nil {
		log.Printf("[UI/DataGrid] Warning: BusinessPool is nil; cannot load master buffer for data_grid at area %q", node.Area)
		return
	}
	log.Printf("[UI/DataGrid] Requesting master buffer eagerly for area %q. SQL: %q", node.Area, node.MasterDataSource)
	go func() {
		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Master buffer load cancelled before start for area %q", node.Area)
			return
		}
		rows, err := BusinessPool.Query(ctx, node.MasterDataSource)
		if err != nil {
			log.Printf("[UI/DataGrid] Error loading master buffer %q: %v", node.MasterDataSource, err)
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
				log.Printf("[UI/DataGrid] Master buffer load cancelled during row scan for area %q", node.Area)
				return
			}
			vals, err := rows.Values()
			if err != nil {
				log.Printf("[UI/DataGrid] Error scanning master row values: %v", err)
				break
			}
			var stringRow []string
			for _, val := range vals {
				stringRow = append(stringRow, formatValue(val))
			}
			dataRows = append(dataRows, stringRow)
		}

		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Master buffer load cancelled before model write for area %q", node.Area)
			return
		}

		log.Printf("[UI/DataGrid] Master buffer execution successful for area %q. Loaded %d columns, %d rows.", node.Area, len(headers), len(dataRows))

		model.mu.Lock()
		model.masterHeaders = headers
		model.masterRows = dataRows
		model.headers = headers
		model.rows = dataRows
		model.mu.Unlock()

		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)
		}
		table.Refresh()
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
		table.Refresh()
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

	table.Refresh()
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

func fetchGridDataAsync(ctx context.Context, node NodeMeta, model *dataGridModel, table *widget.Table, query string, args ...any) {
	if query == "" {
		return
	}
	log.Printf("[UI/DataGrid] Requesting data async for area %q. SQL: %q, Args: %+v", node.Area, query, args)
	go func() {
		if BusinessPool == nil {
			log.Printf("[UI/DataGrid] Warning: BusinessPool is nil; cannot execute query for data_grid at area %q", node.Area)
			return
		}
		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Query cancelled before start for area %q", node.Area)
			return
		}
		rows, err := BusinessPool.Query(ctx, query, args...)
		if err != nil {
			log.Printf("[UI/DataGrid] Error running query %q: %v", query, err)
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
				log.Printf("[UI/DataGrid] Query cancelled during row scan for area %q", node.Area)
				return
			}
			vals, err := rows.Values()
			if err != nil {
				log.Printf("[UI/DataGrid] Error scanning row values: %v", err)
				break
			}
			var stringRow []string
			for _, val := range vals {
				stringRow = append(stringRow, formatValue(val))
			}
			dataRows = append(dataRows, stringRow)
		}

		if err := ctx.Err(); err != nil {
			log.Printf("[UI/DataGrid] Query cancelled before model write for area %q", node.Area)
			return
		}

		log.Printf("[UI/DataGrid] Query execution successful for area %q. Loaded %d columns, %d rows.", node.Area, len(headers), len(dataRows))

		model.mu.Lock()
		model.headers = headers
		model.columns = headers
		model.rows = dataRows
		model.mu.Unlock()

		for i := 0; i < len(headers); i++ {
			table.SetColumnWidth(i, 150)
		}
		table.Refresh()
	}()
}

func formatValue(val any) string {
	if val == nil {
		return ""
	}
	if valuer, ok := val.(driver.Valuer); ok {
		v, err := valuer.Value()
		if err == nil && v != nil {
			switch ts := v.(type) {
			case []byte:
				return string(ts)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return fmt.Sprintf("%v", val)
}
