package dataaccess_test

import (
	"reflect"
	"testing"

	"GolemUI/pkg/dataaccess"
)

func TestExtractOrderedArgs_NormalExtraction(t *testing.T) {
	// TEA-01: snap={"a":"1","b":"2"}, filterKeys=["a","b"] → []any{"1","2"}
	snap := map[string]any{"a": "1", "b": "2"}
	keys := []string{"a", "b"}
	got := dataaccess.ExtractOrderedArgs(snap, keys)
	want := []any{"1", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractOrderedArgs() = %v, want %v", got, want)
	}
}

func TestExtractOrderedArgs_MissingKey(t *testing.T) {
	// TEA-02: snap={"a":"1"}, filterKeys=["a","b"] → []any{"1",""}
	snap := map[string]any{"a": "1"}
	keys := []string{"a", "b"}
	got := dataaccess.ExtractOrderedArgs(snap, keys)
	want := []any{"1", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractOrderedArgs() = %v, want %v", got, want)
	}
}

func TestExtractOrderedArgs_EmptyFilterKeys(t *testing.T) {
	// TEA-03: snap={"a":"1"}, filterKeys=[] → []any{}
	snap := map[string]any{"a": "1"}
	keys := []string{}
	got := dataaccess.ExtractOrderedArgs(snap, keys)
	if len(got) != 0 {
		t.Errorf("ExtractOrderedArgs() = %v (len %d), want empty non-nil slice", got, len(got))
	}
	if got == nil {
		t.Error("ExtractOrderedArgs() returned nil, want non-nil empty slice")
	}
}

func TestExtractOrderedArgs_NilSnapshot(t *testing.T) {
	// TEA-04: snap=nil, filterKeys=["a"] → []any{""}
	got := dataaccess.ExtractOrderedArgs(nil, []string{"a"})
	want := []any{""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractOrderedArgs(nil, keys) = %v, want %v", got, want)
	}
}

func TestExtractOrderedArgs_OrderingPreserved(t *testing.T) {
	// TEA-05: snap={"b":"2","a":"1","c":"3"}, filterKeys=["c","a","b"] → []any{"3","1","2"}
	snap := map[string]any{"b": "2", "a": "1", "c": "3"}
	keys := []string{"c", "a", "b"}
	got := dataaccess.ExtractOrderedArgs(snap, keys)
	want := []any{"3", "1", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractOrderedArgs() = %v, want %v", got, want)
	}
}

func TestExtractOrderedArgs_NilFilterKeys(t *testing.T) {
	// TEA-06: snap={"a":"1"}, filterKeys=nil → []any{}
	snap := map[string]any{"a": "1"}
	got := dataaccess.ExtractOrderedArgs(snap, nil)
	if len(got) != 0 {
		t.Errorf("ExtractOrderedArgs(snap, nil) = %v (len %d), want empty non-nil slice", got, len(got))
	}
	if got == nil {
		t.Error("ExtractOrderedArgs(snap, nil) returned nil, want non-nil empty slice")
	}
}
