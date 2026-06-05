package ui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"GolemUI/pkg/ui"
)

func TestMain(m *testing.M) {
	test.NewApp()
	m.Run()
}

func TestCompose_SimpleHierarchy(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "root",
		ComponentRef: "container",
		Layout: ui.LayoutMeta{
			Type: "horizontal",
		},
		Children: []ui.NodeMeta{
			{
				Area:         "label_area",
				ComponentRef: "label",
				Label:        "Username:",
			},
			{
				Area:         "input_area",
				ComponentRef: "text_input",
				Placeholder:  "Enter username",
				DefaultValue: "admin",
			},
		},
	}

	obj, err := ui.Compose(node)
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	if len(c.Objects) != 2 {
		t.Errorf("expected 2 child objects, got %d", len(c.Objects))
	}

	lbl, ok := c.Objects[0].(*widget.Label)
	if !ok {
		t.Errorf("expected first child to be *widget.Label, got %T", c.Objects[0])
	} else if lbl.Text != "Username:" {
		t.Errorf("expected label text 'Username:', got %q", lbl.Text)
	}

	entry, ok := c.Objects[1].(*widget.Entry)
	if !ok {
		t.Errorf("expected second child to be *widget.Entry, got %T", c.Objects[1])
	} else {
		if entry.Text != "admin" {
			t.Errorf("expected entry text 'admin', got %q", entry.Text)
		}
		if entry.PlaceHolder != "Enter username" {
			t.Errorf("expected placeholder 'Enter username', got %q", entry.PlaceHolder)
		}
	}
}

func TestCompose_Fallback(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "invalid_node",
		ComponentRef: "non_existent_component_ref",
	}

	obj, err := ui.Compose(node)
	if err != nil {
		t.Fatalf("expected graceful handling without error, got err: %v", err)
	}

	lbl, ok := obj.(*widget.Label)
	if !ok {
		t.Fatalf("expected fallback to be a *widget.Label, got %T", obj)
	}

	if lbl.Text == "" {
		t.Errorf("expected fallback label text to be populated, got empty string")
	}
}

func TestCompose_GridAndButton(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "grid_root",
		ComponentRef: "container",
		Layout: ui.LayoutMeta{
			Type:    "grid",
			Columns: []string{"2fr", "1fr"},
			Rows:    []string{"auto"},
			Gap:     "15",
		},
		Children: []ui.NodeMeta{
			{
				Area:         "btn_area",
				ComponentRef: "button",
				Label:        "Submit",
			},
		},
	}

	obj, err := ui.Compose(node)
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Verify layout is FractionalLayout
	lay, ok := c.Layout.(*ui.FractionalLayout)
	if !ok {
		t.Fatalf("expected grid layout to use *ui.FractionalLayout, got %T", c.Layout)
	}

	if len(lay.Columns) != 2 || lay.Columns[0] != "2fr" || lay.Columns[1] != "1fr" {
		t.Errorf("expected layout columns ['2fr', '1fr'], got %v", lay.Columns)
	}

	if lay.Gap != 15 {
		t.Errorf("expected gap to be 15, got %f", lay.Gap)
	}

	if len(c.Objects) != 1 {
		t.Errorf("expected 1 child object, got %d", len(c.Objects))
	}

	btn, ok := c.Objects[0].(*widget.Button)
	if !ok {
		t.Fatalf("expected child to be *widget.Button, got %T", c.Objects[0])
	}

	if btn.Text != "Submit" {
		t.Errorf("expected button text 'Submit', got %q", btn.Text)
	}
}
