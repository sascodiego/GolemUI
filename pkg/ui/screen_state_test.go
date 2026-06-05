package ui_test

import (
	"sync"
	"testing"

	"GolemUI/pkg/ui"
)

func TestNewScreenState_NotNil(t *testing.T) {
	s := ui.NewScreenState("test-screen")
	if s == nil {
		t.Fatal("expected non-nil ScreenState from NewScreenState()")
	}
}

func TestScreenState_SetAndGet(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("username", "admin")
	got := s.Get("username")
	if got != "admin" {
		t.Errorf("expected Get('username') = 'admin', got %v", got)
	}
}

func TestScreenState_GetMissingKey(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	got := s.Get("nonexistent")
	if got != nil {
		t.Errorf("expected Get('nonexistent') = nil, got %v", got)
	}
}

func TestScreenState_OverwriteExistingKey(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("key", "first")
	s.Set("key", "second")
	got := s.Get("key")
	if got != "second" {
		t.Errorf("expected Get('key') = 'second' (latest wins), got %v", got)
	}
}

func TestScreenState_SnapshotContainsAllKeys(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("name", "Alice")
	s.Set("age", 30)
	s.Set("active", true)

	snap := s.Snapshot()

	if v, ok := snap["name"]; !ok || v != "Alice" {
		t.Errorf("expected snap['name'] = 'Alice', got %v", v)
	}
	if v, ok := snap["age"]; !ok || v != 30 {
		t.Errorf("expected snap['age'] = 30, got %v", v)
	}
	if v, ok := snap["active"]; !ok || v != true {
		t.Errorf("expected snap['active'] = true, got %v", v)
	}
}

func TestScreenState_SnapshotDefensiveCopy(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("key", "original")
	snap := s.Snapshot()

	// Mutate snapshot — must NOT affect the store
	snap["key"] = "mutated"

	got := s.Get("key")
	if got != "original" {
		t.Errorf("snapshot mutation leaked into store: expected 'original', got %v", got)
	}
}

func TestScreenState_SnapshotDefensiveCopy_AddedKey(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("a", "1")
	snap := s.Snapshot()

	// Add key to snapshot — must NOT affect the store
	snap["b"] = "2"

	got := s.Get("b")
	if got != nil {
		t.Errorf("snapshot addition leaked into store: expected nil, got %v", got)
	}
}

func TestScreenState_ConcurrentSet(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			s.Set("counter", val)
		}(i)
	}
	wg.Wait()

	// The final value must be one of the written values (0–99)
	got := s.Get("counter")
	if got == nil {
		t.Fatal("expected counter to have a value after concurrent writes, got nil")
	}
	if _, ok := got.(int); !ok {
		t.Fatalf("expected counter to be int, got %T", got)
	}
	val := got.(int)
	if val < 0 || val >= 100 {
		t.Errorf("expected counter in [0,99], got %d", val)
	}
}

func TestScreenState_ConcurrentSetAndGet(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("key", "initial")

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			s.Set("key", val)
		}(i)
	}
	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Get("key")
		}()
	}
	wg.Wait()
}

func TestScreenState_ConcurrentSetAndSnapshot(t *testing.T) {
	s := ui.NewScreenState("test-screen")

	s.Set("a", "1")
	s.Set("b", "2")

	var wg sync.WaitGroup
	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			s.Set("x", val)
		}(i)
	}
	// Concurrent snapshots
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := s.Snapshot()
			if snap == nil {
				t.Error("expected non-nil snapshot")
			}
		}()
	}
	wg.Wait()
}
