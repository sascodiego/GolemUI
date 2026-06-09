package main

import (
	"testing"
)

// --- parseNavigateTarget tests (Spec 018) ---

func TestParseNavigateTarget_PlainVistaID(t *testing.T) {
	cleanID, params := parseNavigateTarget("simple_vista")
	if cleanID != "simple_vista" {
		t.Errorf("expected cleanID 'simple_vista', got %q", cleanID)
	}
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestParseNavigateTarget_WithParams(t *testing.T) {
	cleanID, params := parseNavigateTarget("detalle?id=99&tipo=debito")
	if cleanID != "detalle" {
		t.Errorf("expected cleanID 'detalle', got %q", cleanID)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	if params["id"] != "99" {
		t.Errorf("expected params['id'] = '99', got %q", params["id"])
	}
	if params["tipo"] != "debito" {
		t.Errorf("expected params['tipo'] = 'debito', got %q", params["tipo"])
	}
}

func TestParseNavigateTarget_EmptyQuery(t *testing.T) {
	cleanID, params := parseNavigateTarget("vista?")
	if cleanID != "vista" {
		t.Errorf("expected cleanID 'vista', got %q", cleanID)
	}
	if params != nil {
		t.Errorf("expected nil params for empty query, got %v", params)
	}
}

func TestParseNavigateTarget_Malformed_EmptyVistaID(t *testing.T) {
	// Empty vistaID before ? → treat as plain string
	cleanID, params := parseNavigateTarget("?foo=bar")
	if params != nil {
		t.Errorf("expected nil params for malformed input, got %v", params)
	}
	if cleanID != "?foo=bar" {
		t.Errorf("expected original string returned, got %q", cleanID)
	}
}

func TestParseNavigateTarget_URLDecodedValues(t *testing.T) {
	cleanID, params := parseNavigateTarget("screen?name=hello%20world&val=a%26b%3Dc")
	if cleanID != "screen" {
		t.Errorf("expected cleanID 'screen', got %q", cleanID)
	}
	if params["name"] != "hello world" {
		t.Errorf("expected params['name'] = 'hello world', got %q", params["name"])
	}
	if params["val"] != "a&b=c" {
		t.Errorf("expected params['val'] = 'a&b=c', got %q", params["val"])
	}
}

func TestParseNavigateTarget_TrailingAmpersand(t *testing.T) {
	cleanID, params := parseNavigateTarget("vista?key=val&")
	if cleanID != "vista" {
		t.Errorf("expected cleanID 'vista', got %q", cleanID)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	if params["key"] != "val" {
		t.Errorf("expected params['key'] = 'val', got %q", params["key"])
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestParseNavigateTarget_EmptyValue(t *testing.T) {
	cleanID, params := parseNavigateTarget("screen?key=")
	if cleanID != "screen" {
		t.Errorf("expected cleanID 'screen', got %q", cleanID)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	if params["key"] != "" {
		t.Errorf("expected params['key'] = '', got %q", params["key"])
	}
}

func TestParseNavigateTarget_NoValueNoEquals(t *testing.T) {
	cleanID, params := parseNavigateTarget("screen?flag")
	if cleanID != "screen" {
		t.Errorf("expected cleanID 'screen', got %q", cleanID)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}
	if params["flag"] != "" {
		t.Errorf("expected params['flag'] = '', got %q", params["flag"])
	}
}

func TestParseNavigateTarget_EmptyString(t *testing.T) {
	cleanID, params := parseNavigateTarget("")
	if cleanID != "" {
		t.Errorf("expected cleanID '', got %q", cleanID)
	}
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestParseNavigateTarget_OnlyQuestionMark(t *testing.T) {
	_, params := parseNavigateTarget("?")
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}
