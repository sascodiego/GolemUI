package ui

import (
	"fmt"
	"sync"
)

// ScreenState holds per-screen input values in a thread-safe map.
// Each screen creates one instance via NewScreenState, threaded through Compose.
type ScreenState struct {
	mu            sync.RWMutex
	data          map[string]any
	submitChannel string
}

// NewScreenState creates an initialized ScreenState scoped to the given vistaID.
func NewScreenState(vistaID string) *ScreenState {
	return &ScreenState{
		data:          make(map[string]any),
		submitChannel: fmt.Sprintf("screen:submit:%s", vistaID),
	}
}

// SubmitChannel returns the scoped submit channel for this screen.
func (s *ScreenState) SubmitChannel() string {
	return s.submitChannel
}

// Set writes a key-value pair into the state map.
func (s *ScreenState) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get reads a value by key. Returns nil if key is absent.
func (s *ScreenState) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

// Preload injects key-value pairs into the state before widget composition.
// Values are only set if the key does not already exist (no-overwrite semantics).
// This is used to pass query-string parameters from Navigate to child widgets.
func (s *ScreenState) Preload(params map[string]any) {
	if len(params) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range params {
		if _, exists := s.data[k]; !exists {
			s.data[k] = v
		}
	}
}

// Snapshot returns a shallow copy of the current state map.
// Mutations to the returned map do not affect the store.
func (s *ScreenState) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]any, len(s.data))
	for k, v := range s.data {
		cp[k] = v
	}
	return cp
}
