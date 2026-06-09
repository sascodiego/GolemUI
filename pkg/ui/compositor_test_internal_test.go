package ui

import (
	"reflect"
	"testing"
)

func TestExtractOrderedArgs_WithFilterKeys(t *testing.T) {
	snap := map[string]any{
		"author": "Asimov",
		"title":  "Foundation",
		"year":   1951,
	}
	keys := []string{"title", "author", "year"}

	args := extractOrderedArgs(snap, keys)

	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[0] != "Foundation" {
		t.Errorf("expected args[0] = 'Foundation', got %v", args[0])
	}
	if args[1] != "Asimov" {
		t.Errorf("expected args[1] = 'Asimov', got %v", args[1])
	}
	if args[2] != 1951 {
		t.Errorf("expected args[2] = 1951, got %v", args[2])
	}
}

func TestExtractOrderedArgs_WithFilterKeys_PartialKeys(t *testing.T) {
	snap := map[string]any{
		"author": "Asimov",
		"title":  "Foundation",
		"extra":  "ignored",
	}
	keys := []string{"title", "author"}

	args := extractOrderedArgs(snap, keys)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "Foundation" {
		t.Errorf("expected args[0] = 'Foundation', got %v", args[0])
	}
	if args[1] != "Asimov" {
		t.Errorf("expected args[1] = 'Asimov', got %v", args[1])
	}
}

func TestExtractOrderedArgs_NoFilterKeys_ReturnsEmpty(t *testing.T) {
	snap := map[string]any{
		"z_key": "last",
		"a_key": "first",
		"m_key": "middle",
	}

	args := extractOrderedArgs(snap, nil)

	if len(args) != 0 {
		t.Fatalf("expected 0 args when no filter keys, got %d", len(args))
	}
}

func TestExtractOrderedArgs_EmptySnapshot(t *testing.T) {
	args := extractOrderedArgs(map[string]any{}, nil)
	if len(args) != 0 {
		t.Errorf("expected 0 args for empty snapshot, got %d", len(args))
	}
}

func TestExtractOrderedArgs_MissingKeyReturnsEmptyString(t *testing.T) {
	snap := map[string]any{
		"author": "Asimov",
	}
	keys := []string{"title", "author"}

	args := extractOrderedArgs(snap, keys)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "" {
		t.Errorf("expected args[0] = \"\" (missing 'title'), got %v", args[0])
	}
	if args[1] != "Asimov" {
		t.Errorf("expected args[1] = 'Asimov', got %v", args[1])
	}
}

// --- resolvePath tests ---

func TestResolvePath(t *testing.T) {
	nestedData := map[string]any{
		"transaccion": map[string]any{
			"id": 101,
			"detalles": map[string]any{
				"moneda": "USD",
				"valor":  500.0,
			},
		},
	}

	tests := []struct {
		name     string
		data     any
		path     string
		expected any
	}{
		{
			name:     "nested 3-level float",
			data:     nestedData,
			path:     "transaccion.detalles.valor",
			expected: 500.0,
		},
		{
			name:     "nested 3-level string",
			data:     nestedData,
			path:     "transaccion.detalles.moneda",
			expected: "USD",
		},
		{
			name:     "nested 2-level int",
			data:     nestedData,
			path:     "transaccion.id",
			expected: 101,
		},
		{
			name:     "map subtree",
			data:     nestedData,
			path:     "transaccion",
			expected: map[string]any{"id": 101, "detalles": map[string]any{"moneda": "USD", "valor": 500.0}},
		},
		{
			name:     "missing key",
			data:     nestedData,
			path:     "transaccion.inexistente",
			expected: nil,
		},
		{
			name:     "missing nested key",
			data:     nestedData,
			path:     "transaccion.detalles.inexistente",
			expected: nil,
		},
		{
			name:     "nil data",
			data:     nil,
			path:     "foo.bar",
			expected: nil,
		},
		{
			name:     "empty path",
			data:     nestedData,
			path:     "",
			expected: nil,
		},
		{
			name:     "non-map intermediate",
			data:     map[string]any{"a": "scalar"},
			path:     "a.b.c",
			expected: nil,
		},
		{
			name:     "single-level key found",
			data:     map[string]any{"name": "test"},
			path:     "name",
			expected: "test",
		},
		{
			name:     "single-level key missing",
			data:     map[string]any{"name": "test"},
			path:     "other",
			expected: nil,
		},
		{
			name:     "boolean leaf",
			data:     map[string]any{"flag": true},
			path:     "flag",
			expected: true,
		},
		{
			name:     "nil leaf value",
			data:     map[string]any{"key": nil},
			path:     "key",
			expected: nil,
		},
		{
			name:     "empty map",
			data:     map[string]any{},
			path:     "anything",
			expected: nil,
		},
		{
			name:     "scalar through non-map chain",
			data:     nestedData,
			path:     "transaccion.id.nombre",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePath(tt.data, tt.path)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("resolvePath(%v, %q) = %v (%T), want %v (%T)",
					tt.data, tt.path, got, got, tt.expected, tt.expected)
			}
		})
	}
}

// --- renderTemplate tests ---

func TestRenderTemplate(t *testing.T) {
	nestedData := map[string]any{
		"transaccion": map[string]any{
			"id": 101,
			"detalles": map[string]any{
				"moneda": "USD",
				"valor":  500.0,
			},
		},
	}

	tests := []struct {
		name     string
		tmpl     string
		data     map[string]any
		expected string
	}{
		{
			name:     "multi-token with scalars",
			tmpl:     "Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}",
			data:     nestedData,
			expected: "Monto: 500 USD",
		},
		{
			name:     "single token",
			tmpl:     "ID: {transaccion.id}",
			data:     nestedData,
			expected: "ID: 101",
		},
		{
			name:     "no tokens",
			tmpl:     "Sin tokens",
			data:     nestedData,
			expected: "Sin tokens",
		},
		{
			name:     "missing token preserved",
			tmpl:     "Missing: {no.existe}",
			data:     nestedData,
			expected: "Missing: {no.existe}",
		},
		{
			name:     "empty template",
			tmpl:     "",
			data:     nestedData,
			expected: "",
		},
		{
			name:     "empty braces preserved",
			tmpl:     "Value: {}",
			data:     map[string]any{"x": 1},
			expected: "Value: {}",
		},
		{
			name:     "whitespace in braces trimmed",
			tmpl:     "Value: { x }",
			data:     map[string]any{"x": 1},
			expected: "Value: 1",
		},
		{
			name:     "adjacent tokens",
			tmpl:     "{a}{b}",
			data:     map[string]any{"a": "X", "b": "Y"},
			expected: "XY",
		},
		{
			name:     "mixed literal and tokens",
			tmpl:     "A {a} B {b} C",
			data:     map[string]any{"a": "1", "b": "2"},
			expected: "A 1 B 2 C",
		},
		{
			name:     "unclosed brace",
			tmpl:     "Literal {",
			data:     nestedData,
			expected: "Literal {",
		},
		{
			name:     "unclosed brace with text after",
			tmpl:     "Start {middle end",
			data:     map[string]any{"middle": "X"},
			expected: "Start {middle end",
		},
		{
			name:     "nil data preserves tokens",
			tmpl:     "{a}",
			data:     nil,
			expected: "{a}",
		},
		{
			name:     "empty data map preserves tokens",
			tmpl:     "{a}",
			data:     map[string]any{},
			expected: "{a}",
		},
		{
			name:     "repeated token",
			tmpl:     "{a} and {a}",
			data:     map[string]any{"a": "X"},
			expected: "X and X",
		},
		{
			name:     "boolean value",
			tmpl:     "Active: {active}",
			data:     map[string]any{"active": true},
			expected: "Active: true",
		},
		{
			name:     "integer value",
			tmpl:     "Count: {n}",
			data:     map[string]any{"n": 42},
			expected: "Count: 42",
		},
		{
			name:     "float value",
			tmpl:     "Rate: {r}",
			data:     map[string]any{"r": 3.14},
			expected: "Rate: 3.14",
		},
		{
			name:     "whitespace-only braces preserved",
			tmpl:     "Value: { }",
			data:     map[string]any{"x": 1},
			expected: "Value: { }",
		},
		{
			name:     "resolved and unresolved mixed",
			tmpl:     "{a} {b} {a}",
			data:     map[string]any{"a": "X"},
			expected: "X {b} X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderTemplate(tt.tmpl, tt.data)
			if got != tt.expected {
				t.Errorf("renderTemplate(%q, ...) = %q, want %q", tt.tmpl, got, tt.expected)
			}
		})
	}
}

// --- parseChannelName tests ---

func TestParseChannelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "literal channel name",
			input:    "publish_selection",
			expected: "publish_selection",
		},
		{
			name:     "event prefix stripped",
			input:    "event:custom_channel",
			expected: "custom_channel",
		},
		{
			name:     "screen:submit passthrough",
			input:    "screen:submit:vista_1",
			expected: "screen:submit:vista_1",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "event prefix with empty remainder",
			input:    "event:",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChannelName(tt.input)
			if got != tt.expected {
				t.Errorf("parseChannelName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"Foundation", "found", true},
		{"Foundation", "FOUND", true},
		{"FOUNDATION", "ation", true},
		{"Hello World", "world", true},
		{"Hello World", "xyz", false},
		{"", "", true},
		{"abc", "", true},
		{"", "a", false},
		{"Asimov", "Asimov", true},
		{"asimov", "Asimov", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsIgnoreCase(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}
