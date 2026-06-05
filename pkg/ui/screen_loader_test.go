package ui_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
)

func TestLoadScreen(t *testing.T) {
	validJSONB := `{"area":"home_root","component_ref":"container","layout":{"type":"vertical"},"children":[{"area":"header","component_ref":"label","label":"Welcome to GolemUI Desktop Client"}]}`

	tests := []struct {
		name      string
		pool      db.DatabasePool
		vistaID   string
		setupMock func(*db.MockDBPool)
		wantErr   bool
		validate  func(t *testing.T, node ui.NodeMeta)
	}{
		{
			name:    "happy path: valid JSONB returns NodeMeta tree",
			vistaID: "home",
			setupMock: func(m *db.MockDBPool) {
				m.RegisterQuery(
					"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
					[]string{"config_columnas"},
					[][]any{{validJSONB}},
					nil,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, node ui.NodeMeta) {
				if node.Area != "home_root" {
					t.Errorf("expected Area %q, got %q", "home_root", node.Area)
				}
				if node.ComponentRef != "container" {
					t.Errorf("expected ComponentRef %q, got %q", "container", node.ComponentRef)
				}
				if node.Layout.Type != "vertical" {
					t.Errorf("expected Layout.Type %q, got %q", "vertical", node.Layout.Type)
				}
				if len(node.Children) != 1 {
					t.Fatalf("expected 1 child, got %d", len(node.Children))
				}
				if node.Children[0].ComponentRef != "label" {
					t.Errorf("expected child ComponentRef %q, got %q", "label", node.Children[0].ComponentRef)
				}
				if node.Children[0].Label != "Welcome to GolemUI Desktop Client" {
					t.Errorf("expected child Label %q, got %q", "Welcome to GolemUI Desktop Client", node.Children[0].Label)
				}
			},
		},
		{
			name:    "missing vista: pgx.ErrNoRows returns descriptive error",
			vistaID: "nonexistent",
			setupMock: func(m *db.MockDBPool) {
				m.RegisterQuery(
					"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
					[]string{"config_columnas"},
					nil,
					pgx.ErrNoRows,
				)
			},
			wantErr: true,
			validate: func(t *testing.T, node ui.NodeMeta) {
				t.Helper()
			},
		},
		{
			name:    "malformed JSONB: returns parse error with context",
			vistaID: "broken",
			setupMock: func(m *db.MockDBPool) {
				m.RegisterQuery(
					"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
					[]string{"config_columnas"},
					[][]any{{`{bad json`}},
					nil,
				)
			},
			wantErr: true,
			validate: func(t *testing.T, node ui.NodeMeta) {
				t.Helper()
			},
		},
		{
			name:     "nil pool: returns error without DB call",
			pool:     nil,
			vistaID:  "home",
			setupMock: func(m *db.MockDBPool) {},
			wantErr:  true,
			validate: func(t *testing.T, node ui.NodeMeta) {
				t.Helper()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.pool
			if tt.setupMock != nil && pool == nil {
				// Only create a mock if pool is explicitly nil for the nil-pool test
				if tt.name == "nil pool: returns error without DB call" {
					// pool stays nil — that's the test
				} else {
					mock := db.NewMockDBPool()
					tt.setupMock(mock)
					pool = mock
				}
			} else if pool == nil {
				mock := db.NewMockDBPool()
				tt.setupMock(mock)
				pool = mock
			}

			node, err := ui.LoadScreen(context.Background(), pool, tt.vistaID)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadScreen() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				tt.validate(t, node)
			}
		})
	}
}

func TestLoadScreen_MissingVistaErrorMessage(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
		[]string{"config_columnas"},
		nil,
		pgx.ErrNoRows,
	)

	_, err := ui.LoadScreen(context.Background(), mock, "missing_screen")
	if err == nil {
		t.Fatal("expected error for missing vista, got nil")
	}

	expected := fmt.Sprintf("LoadScreen: vista %q not found", "missing_screen")
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

func TestLoadScreen_MalformedJSONBErrorType(t *testing.T) {
	mock := db.NewMockDBPool()
	mock.RegisterQuery(
		"SELECT config_columnas FROM golemui.vistas_consulta WHERE id = $1",
		[]string{"config_columnas"},
		[][]any{{`{invalid`}},
		nil,
	)

	_, err := ui.LoadScreen(context.Background(), mock, "broken")
	if err == nil {
		t.Fatal("expected error for malformed JSONB, got nil")
	}

	// Verify the error wraps a json.SyntaxError
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("expected error to wrap json.SyntaxError, got: %v (type: %T)", err, err)
	}
}

func TestLoadScreen_NilPoolErrorMessage(t *testing.T) {
	_, err := ui.LoadScreen(context.Background(), nil, "home")
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}

	expected := "LoadScreen: pool is nil"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}
