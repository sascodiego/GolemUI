package dataaccess_test

import (
	"database/sql/driver"
	"fmt"
	"testing"

	"GolemUI/pkg/dataaccess"
)

// mockValuer implements driver.Valuer for testing.
type mockValuer struct {
	val any
	err error
}

func (m *mockValuer) Value() (driver.Value, error) {
	return m.val, m.err
}

var _ driver.Valuer = (*mockValuer)(nil)

func TestFormatValue_Nil(t *testing.T) {
	// TFV-01: FormatValue(nil) → ""
	got := dataaccess.FormatValue(nil)
	if got != "" {
		t.Errorf("FormatValue(nil) = %q, want %q", got, "")
	}
}

func TestFormatValue_Int(t *testing.T) {
	// TFV-02: FormatValue(42) → "42"
	got := dataaccess.FormatValue(42)
	if got != "42" {
		t.Errorf("FormatValue(42) = %q, want %q", got, "42")
	}
}

func TestFormatValue_Float(t *testing.T) {
	// TFV-03: FormatValue(3.14) → "3.14"
	got := dataaccess.FormatValue(3.14)
	if got != "3.14" {
		t.Errorf("FormatValue(3.14) = %q, want %q", got, "3.14")
	}
}

func TestFormatValue_String(t *testing.T) {
	// TFV-04: FormatValue("hello") → "hello"
	got := dataaccess.FormatValue("hello")
	if got != "hello" {
		t.Errorf("FormatValue(%q) = %q, want %q", "hello", got, "hello")
	}
}

func TestFormatValue_Bool(t *testing.T) {
	// TFV-05: FormatValue(true) → "true"
	got := dataaccess.FormatValue(true)
	if got != "true" {
		t.Errorf("FormatValue(true) = %q, want %q", got, "true")
	}
}

func TestFormatValue_ValuerByteSlice(t *testing.T) {
	// TFV-06: FormatValue(&mockValuer{val: []byte("data")}) → "data"
	mv := &mockValuer{val: []byte("data")}
	got := dataaccess.FormatValue(mv)
	if got != "data" {
		t.Errorf("FormatValue(valuer with []byte) = %q, want %q", got, "data")
	}
}

func TestFormatValue_ValuerString(t *testing.T) {
	// TFV-07: FormatValue(&mockValuer{val: "text"}) → "text"
	mv := &mockValuer{val: "text"}
	got := dataaccess.FormatValue(mv)
	if got != "text" {
		t.Errorf("FormatValue(valuer with string) = %q, want %q", got, "text")
	}
}

func TestFormatValue_ValuerNil(t *testing.T) {
	// TFV-08: FormatValue(&mockValuer{val: nil}) → falls through to fmt.Sprintf("%v", mockValuer)
	mv := &mockValuer{val: nil}
	got := dataaccess.FormatValue(mv)
	want := fmt.Sprintf("%v", mv)
	if got != want {
		t.Errorf("FormatValue(valuer returning nil) = %q, want %q", got, want)
	}
}

func TestFormatValue_ValuerError(t *testing.T) {
	// TFV-09: FormatValue(&mockValuer{err: fmt.Errorf("fail")}) → falls through
	mv := &mockValuer{err: fmt.Errorf("fail")}
	got := dataaccess.FormatValue(mv)
	want := fmt.Sprintf("%v", mv)
	if got != want {
		t.Errorf("FormatValue(valuer returning error) = %q, want %q", got, want)
	}
}
