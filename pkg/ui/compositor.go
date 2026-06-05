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
	mu          sync.RWMutex
	headers     []string
	columns     []string
	rows        [][]string
	cancel      context.CancelFunc
	unsubscribe func()
}

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

func Compose(node NodeMeta) (fyne.CanvasObject, error) {
	switch node.ComponentRef {
	case "container":
		var objects []fyne.CanvasObject
		for _, child := range node.Children {
			cObj, err := Compose(child)
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
		if node.BindTo != "" && LocalEventBus != nil {
			entry.OnChanged = func(text string) {
				LocalEventBus.Publish(node.BindTo, text)
			}
		}
		return entry, nil

	case "button":
		return widget.NewButton(node.Label, func() {}), nil

	case "data_grid":
		model := &dataGridModel{}
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

		fetchGridDataAsync(ctx, node, model, table)

		if node.BindTo != "" && LocalEventBus != nil {
			subID := LocalEventBus.Subscribe(node.BindTo, func(ev eventbus.Event) {
				model.mu.Lock()
				if model.cancel != nil {
					model.cancel()
				}
				subCtx, subCancel := context.WithCancel(context.Background())
				model.cancel = subCancel
				model.mu.Unlock()

				fetchGridDataAsync(subCtx, node, model, table, ev.Payload)
			})
			model.mu.Lock()
			model.unsubscribe = func() {
				LocalEventBus.Unsubscribe(node.BindTo, subID)
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
