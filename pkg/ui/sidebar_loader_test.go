package ui_test

import (
	"context"
	"strings"
	"testing"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
)

const navQuery = "SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id"

func TestNavigationMenuQuery(t *testing.T) {
	expected := "SELECT id, padre_id, titulo, vista_id, orden FROM golemui.menu_navegacion ORDER BY padre_id NULLS FIRST, orden, id"
	if ui.NavigationMenuQuery != expected {
		t.Errorf("expected NavigationMenuQuery %q, got %q", expected, ui.NavigationMenuQuery)
	}
}

func TestLoadNavigationMenu_ValidHierarchy(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		navQuery,
		[]string{"id", "padre_id", "titulo", "vista_id", "orden"},
		[][]any{
			{"nav_principal", nil, "Menú Principal", nil, 0},
			{"nav_home", strPtr("nav_principal"), "Inicio", strPtr("home"), 1},
			{"nav_transacciones", strPtr("nav_principal"), "Transacciones", strPtr("transacciones_list"), 2},
			{"nav_query_runner", strPtr("nav_principal"), "Consola SQL", strPtr("query_runner"), 3},
		},
		nil,
	)

	items, err := ui.LoadNavigationMenu(context.Background(), mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	// Verify root
	root := items[0]
	if root.ID != "nav_principal" {
		t.Errorf("expected root ID %q, got %q", "nav_principal", root.ID)
	}
	if root.PadreID != "" {
		t.Errorf("expected root PadreID empty, got %q", root.PadreID)
	}
	if root.VistaID != "" {
		t.Errorf("expected root VistaID empty, got %q", root.VistaID)
	}

	// Verify children sorted by orden
	if items[1].ID != "nav_home" {
		t.Errorf("expected items[1].ID %q, got %q", "nav_home", items[1].ID)
	}
	if items[1].PadreID != "nav_principal" {
		t.Errorf("expected items[1].PadreID %q, got %q", "nav_principal", items[1].PadreID)
	}
	if items[1].VistaID != "home" {
		t.Errorf("expected items[1].VistaID %q, got %q", "home", items[1].VistaID)
	}

	if items[2].ID != "nav_transacciones" {
		t.Errorf("expected items[2].ID %q, got %q", "nav_transacciones", items[2].ID)
	}
	if items[3].ID != "nav_query_runner" {
		t.Errorf("expected items[3].ID %q, got %q", "nav_query_runner", items[3].ID)
	}
}

func TestLoadNavigationMenu_CycleDetected(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		navQuery,
		[]string{"id", "padre_id", "titulo", "vista_id", "orden"},
		[][]any{
			{"B", strPtr("A"), "Node B", nil, 0},
			{"A", strPtr("B"), "Node A", nil, 1},
		},
		nil,
	)

	_, err := ui.LoadNavigationMenu(context.Background(), mock)
	if err == nil {
		t.Fatal("expected error for cyclic hierarchy, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "cycle detected") {
		t.Errorf("expected error to contain %q, got %q", "cycle detected", errMsg)
	}
}

func TestLoadNavigationMenu_SelfLoop(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		navQuery,
		[]string{"id", "padre_id", "titulo", "vista_id", "orden"},
		[][]any{
			{"X", strPtr("X"), "Self-loop", nil, 0},
		},
		nil,
	)

	_, err := ui.LoadNavigationMenu(context.Background(), mock)
	if err == nil {
		t.Fatal("expected error for self-loop, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "cycle detected: X → X") {
		t.Errorf("expected error to contain %q, got %q", "cycle detected: X → X", errMsg)
	}
}

func TestLoadNavigationMenu_NilPool(t *testing.T) {
	_, err := ui.LoadNavigationMenu(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}

	expected := "LoadNavigationMenu: pool is nil"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestLoadNavigationMenu_EmptyResult(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		navQuery,
		[]string{"id", "padre_id", "titulo", "vista_id", "orden"},
		[][]any{},
		nil,
	)

	items, err := ui.LoadNavigationMenu(context.Background(), mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if items == nil {
		t.Fatal("expected non-nil slice for empty result, got nil")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestLoadNavigationMenu_ThreeNodeCycle(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		navQuery,
		[]string{"id", "padre_id", "titulo", "vista_id", "orden"},
		[][]any{
			{"A", strPtr("C"), "Node A", nil, 0},
			{"B", strPtr("A"), "Node B", nil, 1},
			{"C", strPtr("B"), "Node C", nil, 2},
		},
		nil,
	)

	_, err := ui.LoadNavigationMenu(context.Background(), mock)
	if err == nil {
		t.Fatal("expected error for 3-node cycle, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "cycle detected") {
		t.Errorf("expected error to contain %q, got %q", "cycle detected", errMsg)
	}
}

// strPtr returns a pointer to the given string value.
// Used in mock row data to provide *string values for nullable columns.
func strPtr(s string) *string {
	return &s
}
