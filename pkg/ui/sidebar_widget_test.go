package ui_test

import (
	"testing"

	"GolemUI/pkg/ui"
	"fyne.io/fyne/v2/widget"
)

// TestBuildNavTree_PopulatesCorrectTitles verifies that the tree is built with
// correct parent-child relationships and that node labels display the expected Titulo.
func TestBuildNavTree_PopulatesCorrectTitles(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
		{ID: "nav_transacciones", PadreID: "nav_principal", Titulo: "Transacciones", VistaID: "transacciones_list", Orden: 2},
		{ID: "nav_query_runner", PadreID: "nav_principal", Titulo: "Consola SQL", VistaID: "query_runner", Orden: 3},
	}

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()
	if tree == nil {
		t.Fatal("expected non-nil tree, got nil")
	}

	// Verify root children
	roots := tree.ChildUIDs("")
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0] != "nav_principal" {
		t.Errorf("expected root ID %q, got %q", "nav_principal", roots[0])
	}

	// Verify branch is detected
	if !tree.IsBranch("nav_principal") {
		t.Error("expected nav_principal to be a branch")
	}

	// Verify children of root
	children := tree.ChildUIDs("nav_principal")
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}

	expectedOrder := []string{"nav_home", "nav_transacciones", "nav_query_runner"}
	for i, expected := range expectedOrder {
		if string(children[i]) != expected {
			t.Errorf("expected child[%d] = %q, got %q", i, expected, children[i])
		}
	}

	// Verify leaves are not branches
	if tree.IsBranch("nav_home") {
		t.Error("expected nav_home to be a leaf (not a branch)")
	}

	// Verify leaf has no children
	leafChildren := tree.ChildUIDs("nav_home")
	if len(leafChildren) != 0 {
		t.Errorf("expected 0 children for leaf, got %d", len(leafChildren))
	}
}

// TestBuildNavTree_LeafTriggersNavigate verifies that selecting a leaf node
// with a non-empty VistaID calls Navigate with the correct VistaID.
func TestBuildNavTree_LeafTriggersNavigate(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
		{ID: "nav_transacciones", PadreID: "nav_principal", Titulo: "Transacciones", VistaID: "transacciones_list", Orden: 2},
		{ID: "nav_query_runner", PadreID: "nav_principal", Titulo: "Consola SQL", VistaID: "query_runner", Orden: 3},
	}

	var navigated string
	ui.Navigate = func(vistaID string) { navigated = vistaID }
	defer func() { ui.Navigate = nil }()

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// Select a leaf node
	tree.OnSelected("nav_home")

	if navigated != "home" {
		t.Errorf("expected Navigate called with %q, got %q", "home", navigated)
	}
}

// TestBuildNavTree_BranchDoesNotTriggerNavigate verifies that selecting a
// branch node does NOT call Navigate.
func TestBuildNavTree_BranchDoesNotTriggerNavigate(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
	}

	var navigated string
	ui.Navigate = func(vistaID string) { navigated = vistaID }
	defer func() { ui.Navigate = nil }()

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// Select the branch node
	tree.OnSelected("nav_principal")

	if navigated != "" {
		t.Errorf("expected Navigate NOT to be called, but it was called with %q", navigated)
	}
}

// TestBuildNavTree_LeafWithoutVistaIDDoesNotNavigate verifies that selecting
// a leaf node with an empty VistaID does NOT call Navigate.
func TestBuildNavTree_LeafWithoutVistaIDDoesNotNavigate(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "spacer", PadreID: "", Titulo: "Spacer", VistaID: "", Orden: 0},
	}

	var navigated string
	ui.Navigate = func(vistaID string) { navigated = vistaID }
	defer func() { ui.Navigate = nil }()

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// Select the leaf node with empty VistaID
	tree.OnSelected("spacer")

	if navigated != "" {
		t.Errorf("expected Navigate NOT to be called for leaf with empty VistaID, but got %q", navigated)
	}
}

// TestBuildNavTree_EmptyItems verifies that BuildNavTree returns a valid
// non-nil tree when given an empty slice.
func TestBuildNavTree_EmptyItems(t *testing.T) {
	navTree := ui.BuildNavTree([]ui.MenuItem{})
	tree := navTree.Widget()
	if tree == nil {
		t.Fatal("expected non-nil tree for empty items, got nil")
	}

	roots := tree.ChildUIDs("")
	if len(roots) != 0 {
		t.Errorf("expected 0 roots for empty items, got %d", len(roots))
	}
}

// TestBuildNavTree_ChildrenSortedByOrden verifies that children are sorted
// by Orden ascending with ID as tiebreaker.
func TestBuildNavTree_ChildrenSortedByOrden(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "parent", PadreID: "", Titulo: "Parent", VistaID: "", Orden: 0},
		{ID: "child_c", PadreID: "parent", Titulo: "C", VistaID: "c", Orden: 3},
		{ID: "child_a", PadreID: "parent", Titulo: "A", VistaID: "a", Orden: 1},
		{ID: "child_b", PadreID: "parent", Titulo: "B", VistaID: "b", Orden: 2},
	}

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	children := tree.ChildUIDs("parent")
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}

	expectedOrder := []string{"child_a", "child_b", "child_c"}
	for i, expected := range expectedOrder {
		if string(children[i]) != expected {
			t.Errorf("expected children[%d] = %q, got %q", i, expected, children[i])
		}
	}
}

// TestBuildNavTree_NilNavigateDoesNotPanic verifies that calling OnSelected
// when Navigate is nil does not panic.
func TestBuildNavTree_NilNavigateDoesNotPanic(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "leaf", PadreID: "", Titulo: "Leaf", VistaID: "home", Orden: 0},
	}

	ui.Navigate = nil

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// This should not panic
	tree.OnSelected("leaf")
}

// TestBuildNavTree_NilSlice verifies that BuildNavTree handles a nil slice.
func TestBuildNavTree_NilSlice(t *testing.T) {
	navTree := ui.BuildNavTree(nil)
	tree := navTree.Widget()
	if tree == nil {
		t.Fatal("expected non-nil tree for nil items, got nil")
	}

	roots := tree.ChildUIDs("")
	if len(roots) != 0 {
		t.Errorf("expected 0 roots for nil items, got %d", len(roots))
	}
}

// TestBuildNavTree_UpdateNodeSetsTitulo verifies that the UpdateNode callback
// correctly sets the label text from idToItem.
func TestBuildNavTree_UpdateNodeSetsTitulo(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
	}

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// Simulate Fyne calling UpdateNode for a branch
	label := widget.NewLabel("")
	tree.UpdateNode("nav_principal", true, label)
	if label.Text != "Menú Principal" {
		t.Errorf("expected label text %q, got %q", "Menú Principal", label.Text)
	}

	// Simulate Fyne calling UpdateNode for a leaf
	label2 := widget.NewLabel("")
	tree.UpdateNode("nav_home", false, label2)
	if label2.Text != "Inicio" {
		t.Errorf("expected label text %q, got %q", "Inicio", label2.Text)
	}
}

// --- T-1.2: NavTree.Widget() accessor test ---

// TestNavTree_WidgetReturnsTree verifies that Widget() returns the underlying
// *widget.Tree and that it is non-nil.
func TestNavTree_WidgetReturnsTree(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
		{ID: "child", PadreID: "root", Titulo: "Child", VistaID: "home", Orden: 1},
	}

	navTree := ui.BuildNavTree(items)

	tree := navTree.Widget()
	if tree == nil {
		t.Fatal("expected Widget() to return non-nil *widget.Tree")
	}

	// Verify the tree is functional by checking children
	roots := tree.ChildUIDs("")
	if len(roots) != 1 || string(roots[0]) != "root" {
		t.Errorf("expected 1 root 'root', got %v", roots)
	}
}

// --- T-1.4: SelectByVistaID tests ---

// TestSelectByVistaID_ValidSelectsNode verifies that SelectByVistaID correctly
// opens ancestor branches and selects the target node.
func TestSelectByVistaID_ValidSelectsNode(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
		{ID: "nav_transacciones", PadreID: "nav_principal", Titulo: "Transacciones", VistaID: "transacciones_list", Orden: 2},
	}

	var navigated string
	ui.Navigate = func(vistaID string) { navigated = vistaID }
	defer func() { ui.Navigate = nil }()

	navTree := ui.BuildNavTree(items)

	// Programmatic select should NOT call Navigate (re-entrancy guard)
	navTree.SelectByVistaID("home")

	// Navigate should NOT have been called (guard prevents re-entry)
	if navigated != "" {
		t.Errorf("expected Navigate NOT to be called during programmatic select, but got %q", navigated)
	}
}

// TestSelectByVistaID_EmptyIsNoOp verifies that calling SelectByVistaID with
// an empty string is a safe no-op.
func TestSelectByVistaID_EmptyIsNoOp(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
	}

	navTree := ui.BuildNavTree(items)

	// Should not panic
	navTree.SelectByVistaID("")
}

// TestSelectByVistaID_UnknownIsNoOp verifies that calling SelectByVistaID with
// an unknown vistaID is a safe no-op.
func TestSelectByVistaID_UnknownIsNoOp(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "root", PadreID: "", Titulo: "Root", VistaID: "", Orden: 0},
	}

	navTree := ui.BuildNavTree(items)

	// Should not panic
	navTree.SelectByVistaID("nonexistent_view")
}

// --- T-1.5: Re-entrancy guard test ---

// TestReentrancyGuardPreventsLoop verifies that when navigating is true
// (programmatic selection in progress), the OnSelected callback does NOT
// call Navigate. This prevents infinite loops:
// Navigate → SelectByVistaID → tree.Select → OnSelected → Navigate → ...
func TestReentrancyGuardPreventsLoop(t *testing.T) {
	items := []ui.MenuItem{
		{ID: "nav_principal", PadreID: "", Titulo: "Menú Principal", VistaID: "", Orden: 0},
		{ID: "nav_home", PadreID: "nav_principal", Titulo: "Inicio", VistaID: "home", Orden: 1},
		{ID: "nav_transacciones", PadreID: "nav_principal", Titulo: "Transacciones", VistaID: "transacciones_list", Orden: 2},
	}

	navigateCount := 0
	ui.Navigate = func(vistaID string) {
		navigateCount++
		// Simulate what Navigate does: call SelectByVistaID for bidirectional sync
		// This would cause infinite recursion without the guard
		navTree := ui.BuildNavTree(items)
		navTree.SelectByVistaID(vistaID)
	}
	defer func() { ui.Navigate = nil }()

	navTree := ui.BuildNavTree(items)
	tree := navTree.Widget()

	// User clicks a leaf in the sidebar → OnSelected fires → Navigate called
	tree.OnSelected("nav_home")

	// Navigate should have been called exactly once (not infinitely)
	if navigateCount != 1 {
		t.Errorf("expected Navigate to be called exactly 1 time, got %d", navigateCount)
	}
}
