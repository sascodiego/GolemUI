package ui

import (
	"fmt"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

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
		return entry, nil

	case "button":
		return widget.NewButton(node.Label, func() {}), nil

	default:
		log.Printf("Warning: Unrecognized component type %q at area %q", node.ComponentRef, node.Area)
		fallback := widget.NewLabel(fmt.Sprintf("[Fallback: Unrecognized component type %q]", node.ComponentRef))
		return fallback, nil
	}
}
