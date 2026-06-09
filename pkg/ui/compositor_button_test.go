package ui_test

import (
	"sync"
	"testing"
	"time"

	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// --- Reactive Navigation Button Tests (Spec 018) ---

func TestCompose_Button_ReactiveNav_StartsDisabled(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	ui.Navigate = func(vistaID string) {}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn, ok := obj.(*widget.Button)
	if !ok {
		t.Fatalf("expected *widget.Button, got %T", obj)
	}

	if !btn.Disabled() {
		t.Error("reactive nav button should start disabled")
	}
}

func TestCompose_Button_ReactiveNav_EnablesOnSelection(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	ui.Navigate = func(vistaID string) {}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)
	if !btn.Disabled() {
		t.Fatal("button should start disabled")
	}

	// Publish valid selection
	eb.Publish("publish_selection", map[string]any{"id": 42})
	time.Sleep(200 * time.Millisecond)

	if btn.Disabled() {
		t.Error("button should be enabled after valid selection")
	}
}

func TestCompose_Button_ReactiveNav_DisablesOnDeselection(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	ui.Navigate = func(vistaID string) {}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)

	// Enable first
	eb.Publish("publish_selection", map[string]any{"id": 42})
	time.Sleep(100 * time.Millisecond)
	if btn.Disabled() {
		t.Fatal("button should be enabled after selection")
	}

	// Publish empty payload → deselect
	eb.Publish("publish_selection", map[string]any{})
	time.Sleep(100 * time.Millisecond)

	if !btn.Disabled() {
		t.Error("button should be disabled after deselection (empty payload)")
	}
}

func TestCompose_Button_ReactiveNav_ClickNavigatesWithParams(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	var navigatedTo string
	var navMu sync.Mutex
	ui.Navigate = func(vistaID string) {
		navMu.Lock()
		navigatedTo = vistaID
		navMu.Unlock()
	}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
		ParamMapping: map[string]string{"id": "id"},
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)

	// Publish selection
	eb.Publish("publish_selection", map[string]any{"id": 42})
	time.Sleep(100 * time.Millisecond)

	// Click button
	test.Tap(btn)
	time.Sleep(100 * time.Millisecond)

	navMu.Lock()
	defer navMu.Unlock()
	if navigatedTo != "detalle?id=42" {
		t.Errorf("expected Navigate called with 'detalle?id=42', got %q", navigatedTo)
	}
}

func TestCompose_Button_ReactiveNav_ClickWhileDisabled_NoNavigation(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	var navigated bool
	ui.Navigate = func(vistaID string) {
		navigated = true
	}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)
	if !btn.Disabled() {
		t.Fatal("button should start disabled")
	}

	// Click while disabled — no navigation should happen
	test.Tap(btn)
	time.Sleep(100 * time.Millisecond)

	if navigated {
		t.Error("Navigate should NOT be called when button is disabled (no selection)")
	}
}

func TestCompose_Button_ReactiveNav_NilBus_FallsBackToStatic(t *testing.T) {
	ui.LocalEventBus = nil
	var navigatedTo string
	ui.Navigate = func(vistaID string) {
		navigatedTo = vistaID
	}
	defer func() { ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn, ok := obj.(*widget.Button)
	if !ok {
		t.Fatalf("expected *widget.Button, got %T", obj)
	}

	// Should be enabled (static navigation fallback)
	if btn.Disabled() {
		t.Error("button should be enabled (static navigation) when EventBus is nil")
	}

	// Click should navigate without params
	test.Tap(btn)

	if navigatedTo != "detalle" {
		t.Errorf("expected Navigate('detalle'), got %q", navigatedTo)
	}
}

func TestCompose_Button_ReactiveNav_CleanupUnsubscribes(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	ui.Navigate = func(vistaID string) {}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	_, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Before cleanup: subscriber should exist
	inner := eb.(*eventbus.InMemEventBus)
	if count := inner.SubscriberCount("publish_selection"); count == 0 {
		t.Error("expected at least 1 subscriber before cleanup")
	}

	cleanup()

	// After cleanup: subscriber should be gone
	if count := inner.SubscriberCount("publish_selection"); count != 0 {
		t.Errorf("expected 0 subscribers after cleanup, got %d", count)
	}
}

func TestCompose_Button_ReactiveNav_IdempotentCleanup(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	ui.Navigate = func(vistaID string) {}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
	}

	_, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Call cleanup twice — must not panic
	cleanup()
	cleanup()

	inner := eb.(*eventbus.InMemEventBus)
	if count := inner.SubscriberCount("publish_selection"); count != 0 {
		t.Errorf("expected 0 subscribers after idempotent cleanup, got %d", count)
	}
}

func TestCompose_Button_NavigateWithoutDataSource_StayStatic(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	var navigatedTo string
	ui.Navigate = func(vistaID string) {
		navigatedTo = vistaID
	}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	// Button with navigate: but NO DataSource → static navigation
	node := ui.NodeMeta{
		Area:         "static_btn",
		ComponentRef: "button",
		Label:        "Go",
		SubmitAction: "navigate:somewhere",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)

	// Should be enabled (static)
	if btn.Disabled() {
		t.Error("static navigation button should be enabled")
	}

	test.Tap(btn)

	if navigatedTo != "somewhere" {
		t.Errorf("expected Navigate('somewhere'), got %q", navigatedTo)
	}
}

func TestCompose_Button_ReactiveNav_ParamMappingMultipleParams(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	var navigatedTo string
	var navMu sync.Mutex
	ui.Navigate = func(vistaID string) {
		navMu.Lock()
		navigatedTo = vistaID
		navMu.Unlock()
	}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
		ParamMapping: map[string]string{"id": "id", "tipo": "transaction_type"},
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)

	// Publish selection
	eb.Publish("publish_selection", map[string]any{"id": 99, "transaction_type": "debito"})
	time.Sleep(100 * time.Millisecond)

	test.Tap(btn)
	time.Sleep(100 * time.Millisecond)

	navMu.Lock()
	defer navMu.Unlock()
	// Output is sorted: id=99&tipo=debito
	if navigatedTo != "detalle?id=99&tipo=debito" {
		t.Errorf("expected 'detalle?id=99&tipo=debito', got %q", navigatedTo)
	}
}

func TestCompose_Button_ReactiveNav_EmptyParamMapping_PlainNavigate(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	var navigatedTo string
	var navMu sync.Mutex
	ui.Navigate = func(vistaID string) {
		navMu.Lock()
		navigatedTo = vistaID
		navMu.Unlock()
	}
	defer func() { ui.LocalEventBus = nil; ui.Navigate = nil }()

	node := ui.NodeMeta{
		Area:         "nav_btn",
		ComponentRef: "button",
		Label:        "View Detail",
		DataSource:   "publish_selection",
		SubmitAction: "navigate:detalle",
		// No ParamMapping
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	btn := obj.(*widget.Button)

	eb.Publish("publish_selection", map[string]any{"id": 42})
	time.Sleep(100 * time.Millisecond)

	test.Tap(btn)
	time.Sleep(100 * time.Millisecond)

	navMu.Lock()
	defer navMu.Unlock()
	if navigatedTo != "detalle" {
		t.Errorf("expected plain 'detalle' (no query string), got %q", navigatedTo)
	}
}
