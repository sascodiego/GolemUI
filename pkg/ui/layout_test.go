package ui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"GolemUI/pkg/ui"
)

type mockCanvasObject struct {
	pos     fyne.Position
	size    fyne.Size
	minSize fyne.Size
	visible bool
}

func (m *mockCanvasObject) Size() fyne.Size {
	return m.size
}

func (m *mockCanvasObject) Resize(s fyne.Size) {
	m.size = s
}

func (m *mockCanvasObject) Position() fyne.Position {
	return m.pos
}

func (m *mockCanvasObject) Move(p fyne.Position) {
	m.pos = p
}

func (m *mockCanvasObject) MinSize() fyne.Size {
	return m.minSize
}

func (m *mockCanvasObject) Visible() bool {
	return m.visible
}

func (m *mockCanvasObject) Show() {
	m.visible = true
}

func (m *mockCanvasObject) Hide() {
	m.visible = false
}

func (m *mockCanvasObject) Refresh() {}

func newMockObject(w, h float32) fyne.CanvasObject {
	return &mockCanvasObject{
		minSize: fyne.NewSize(w, h),
		visible: true,
	}
}

func TestFractionalLayout_MinSize(t *testing.T) {
	layout := &ui.FractionalLayout{
		Columns: []string{"2fr", "1fr"},
		Rows:    []string{"1fr"},
		Gap:     10,
	}

	obj1 := newMockObject(100, 20)
	obj2 := newMockObject(40, 30)

	objects := []fyne.CanvasObject{obj1, obj2}
	minSize := layout.MinSize(objects)

	expectedWidth := float32(160)
	expectedHeight := float32(30)

	if minSize.Width != expectedWidth {
		t.Errorf("expected min width %f, got %f", expectedWidth, minSize.Width)
	}
	if minSize.Height != expectedHeight {
		t.Errorf("expected min height %f, got %f", expectedHeight, minSize.Height)
	}
}

func TestFractionalLayout_Triangulate(t *testing.T) {
	layout := &ui.FractionalLayout{
		Columns: []string{"1.5fr", "auto", "100"},
		Rows:    []string{"auto", "2.5fr"},
		Gap:     5,
	}

	obj00 := newMockObject(60, 40)
	obj10 := newMockObject(80, 50)
	obj20 := newMockObject(10, 10)
	obj01 := newMockObject(30, 20)
	obj11 := newMockObject(40, 25)
	obj21 := newMockObject(50, 30)

	objects := []fyne.CanvasObject{obj00, obj10, obj20, obj01, obj11, obj21}

	// 1. Check MinSize
	minSize := layout.MinSize(objects)
	expectedWidth := float32(250) // Col0(60) + Col1(80) + Col2(100) + Gap*2(10)
	expectedHeight := float32(85) // Row0(50) + Row1(30) + Gap*1(5)

	if minSize.Width != expectedWidth {
		t.Errorf("expected min width %f, got %f", expectedWidth, minSize.Width)
	}
	if minSize.Height != expectedHeight {
		t.Errorf("expected min height %f, got %f", expectedHeight, minSize.Height)
	}

	// 2. Check Layout positioning and sizing with extra space
	containerSize := fyne.NewSize(310, 105)
	layout.Layout(objects, containerSize)

	expectedPositions := []fyne.Position{
		fyne.NewPos(0, 0),     // 00
		fyne.NewPos(125, 0),   // 10
		fyne.NewPos(210, 0),   // 20
		fyne.NewPos(0, 55),    // 01
		fyne.NewPos(125, 55),  // 11
		fyne.NewPos(210, 55),  // 21
	}

	expectedSizes := []fyne.Size{
		fyne.NewSize(120, 50), // 00
		fyne.NewSize(80, 50),  // 10
		fyne.NewSize(100, 50), // 20
		fyne.NewSize(120, 50), // 01
		fyne.NewSize(80, 50),  // 11
		fyne.NewSize(100, 50), // 21
	}

	for i, obj := range objects {
		if obj.Position() != expectedPositions[i] {
			t.Errorf("obj %d: expected position %v, got %v", i, expectedPositions[i], obj.Position())
		}
		if obj.Size() != expectedSizes[i] {
			t.Errorf("obj %d: expected size %v, got %v", i, expectedSizes[i], obj.Size())
		}
	}
}
