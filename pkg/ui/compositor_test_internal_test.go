package ui

import (
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
