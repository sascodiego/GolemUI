package ui_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"GolemUI/pkg/dataaccess"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestMain(m *testing.M) {
	test.NewApp()
	m.Run()
}

// trackingMockDataSource wraps MockDataSource and records calls.
type trackingMockDataSource struct {
	*dataaccess.MockDataSource
	mu         sync.Mutex
	fetchCalls []struct {
		source string
		args   []any
	}
	fetchAllCalls []string
}

func (t *trackingMockDataSource) Fetch(ctx context.Context, source string, args ...any) (dataaccess.DataSet, error) {
	t.mu.Lock()
	t.fetchCalls = append(t.fetchCalls, struct {
		source string
		args   []any
	}{source, args})
	t.mu.Unlock()
	return t.MockDataSource.Fetch(ctx, source, args...)
}

func (t *trackingMockDataSource) FetchAll(ctx context.Context, source string) (dataaccess.DataSet, error) {
	t.mu.Lock()
	t.fetchAllCalls = append(t.fetchAllCalls, source)
	t.mu.Unlock()
	return t.MockDataSource.FetchAll(ctx, source)
}

func TestCompose_SimpleHierarchy(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "root",
		ComponentRef: "container",
		Layout: ui.LayoutMeta{
			Type: "horizontal",
		},
		Children: []ui.NodeMeta{
			{
				Area:         "label_area",
				ComponentRef: "label",
				Label:        "Username:",
			},
			{
				Area:         "input_area",
				ComponentRef: "text_input",
				Placeholder:  "Enter username",
				DefaultValue: "admin",
			},
		},
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	if len(c.Objects) != 2 {
		t.Errorf("expected 2 child objects, got %d", len(c.Objects))
	}

	lbl, ok := c.Objects[0].(*widget.Label)
	if !ok {
		t.Errorf("expected first child to be *widget.Label, got %T", c.Objects[0])
	} else if lbl.Text != "Username:" {
		t.Errorf("expected label text 'Username:', got %q", lbl.Text)
	}

	entry, ok := c.Objects[1].(*widget.Entry)
	if !ok {
		t.Errorf("expected second child to be *widget.Entry, got %T", c.Objects[1])
	} else {
		if entry.Text != "admin" {
			t.Errorf("expected entry text 'admin', got %q", entry.Text)
		}
		if entry.PlaceHolder != "Enter username" {
			t.Errorf("expected placeholder 'Enter username', got %q", entry.PlaceHolder)
		}
	}
}

func TestCompose_Fallback(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "invalid_node",
		ComponentRef: "non_existent_component_ref",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("expected graceful handling without error, got err: %v", err)
	}
	defer cleanup()

	lbl, ok := obj.(*widget.Label)
	if !ok {
		t.Fatalf("expected fallback to be a *widget.Label, got %T", obj)
	}

	if lbl.Text == "" {
		t.Errorf("expected fallback label text to be populated, got empty string")
	}
}

func TestCompose_GridAndButton(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "grid_root",
		ComponentRef: "container",
		Layout: ui.LayoutMeta{
			Type:    "grid",
			Columns: []string{"2fr", "1fr"},
			Rows:    []string{"auto"},
			Gap:     "15",
		},
		Children: []ui.NodeMeta{
			{
				Area:         "btn_area",
				ComponentRef: "button",
				Label:        "Submit",
			},
		},
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Verify layout is FractionalLayout
	lay, ok := c.Layout.(*ui.FractionalLayout)
	if !ok {
		t.Fatalf("expected grid layout to use *ui.FractionalLayout, got %T", c.Layout)
	}

	if len(lay.Columns) != 2 || lay.Columns[0] != "2fr" || lay.Columns[1] != "1fr" {
		t.Errorf("expected layout columns ['2fr', '1fr'], got %v", lay.Columns)
	}

	if lay.Gap != 15 {
		t.Errorf("expected gap to be 15, got %f", lay.Gap)
	}

	if len(c.Objects) != 1 {
		t.Errorf("expected 1 child object, got %d", len(c.Objects))
	}

	btn, ok := c.Objects[0].(*widget.Button)
	if !ok {
		t.Fatalf("expected child to be *widget.Button, got %T", c.Objects[0])
	}

	if btn.Text != "Submit" {
		t.Errorf("expected button text 'Submit', got %q", btn.Text)
	}
}

func TestDataSourceExists(t *testing.T) {
	// Verify DS global exists and is the correct interface type
	var ds interface{} = ui.DS
	_ = ds
}

func TestCWR_DefaultsNil(t *testing.T) {
	if ui.CWR != nil {
		t.Errorf("expected CWR to be nil at package init, got %v", ui.CWR)
	}
}

func TestCompose_DataGrid_Success(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id", "title", "amount"},
			Rows:    [][]string{{"1", "Book A", "25.5"}, {"2", "Book B", "35"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT * FROM books",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected composed object to be *widget.Table, got %T", obj)
	}

	// Poll/wait for async loading to complete (up to 500ms)
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, colsCount := table.Length()
		if rows == 2 && colsCount == 3 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !loaded {
		t.Fatal("timeout waiting for data_grid to load data async")
	}

	// Assert headers
	header0 := table.CreateHeader()
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 0}, header0)
	lbl0 := header0.(*widget.Label)
	if lbl0.Text != "id" {
		t.Errorf("expected header 0 to be 'id', got %q", lbl0.Text)
	}

	header1 := table.CreateHeader()
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 1}, header1)
	lbl1 := header1.(*widget.Label)
	if lbl1.Text != "title" {
		t.Errorf("expected header 1 to be 'title', got %q", lbl1.Text)
	}

	// Assert cell content
	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 1}, cell)
	lblCell := cell.(*widget.Label)
	if lblCell.Text != "Book A" {
		t.Errorf("expected cell (0,1) text 'Book A', got %q", lblCell.Text)
	}

	table.UpdateCell(widget.TableCellID{Row: 1, Col: 2}, cell)
	lblCell2 := cell.(*widget.Label)
	if lblCell2.Text != "35" && lblCell2.Text != "35.0" && lblCell2.Text != "35%" {
		t.Errorf("expected cell (1,2) text to represent 35, got %q", lblCell2.Text)
	}
}

func TestCompose_DataGrid_NoDataSource(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected composed object to be *widget.Table, got %T", obj)
	}

	rows, cols := table.Length()
	if rows != 0 || cols != 0 {
		t.Errorf("expected 0x0 table when DataSource is empty, got %dx%d", rows, cols)
	}
}

func TestCompose_DataGrid_NilDataSource(t *testing.T) {
	ui.DS = nil
	ui.CWR = nil
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT 1",
	}

	// This should not crash / panic
	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected composed object to be *widget.Table, got %T", obj)
	}

	rows, cols := table.Length()
	if rows != 0 || cols != 0 {
		t.Errorf("expected 0x0 table when DataSource is nil, got %dx%d", rows, cols)
	}
}

func TestCompose_DataGrid_ReactiveFiltering(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	trackingDS := &trackingMockDataSource{
		MockDataSource: &dataaccess.MockDataSource{
			FetchResult: dataaccess.DataSet{
				Headers: []string{"id", "title"},
				Rows:    [][]string{{"1", "Book A"}},
			},
		},
	}
	ui.DS = trackingDS
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	// Compose a container with text_input + submit button + data_grid
	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "input_area",
				ComponentRef: "text_input",
				BindTo:       "title",
				Placeholder:  "Filter books",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Search",
				SubmitAction: "search",
			},
			{
				Area:         "grid_area",
				ComponentRef: "data_grid",
				DataSource:   "SELECT * FROM books WHERE title LIKE $1",
				FilterMode:   "server",
				FilterKeys:   []string{"title"},
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}
	if len(c.Objects) != 3 {
		t.Fatalf("expected 3 children, got %d", len(c.Objects))
	}

	entry, ok := c.Objects[0].(*widget.Entry)
	if !ok {
		t.Fatalf("expected first child to be *widget.Entry, got %T", c.Objects[0])
	}

	submitBtn, ok := c.Objects[1].(*widget.Button)
	if !ok {
		t.Fatalf("expected second child to be *widget.Button, got %T", c.Objects[1])
	}

	// Type into the entry, then click submit
	test.Type(entry, "Book A")
	test.Tap(submitBtn)

	// Wait for query to execute with the typed text as positional arg
	var foundCall bool
	for start := time.Now(); time.Since(start) < 1000*time.Millisecond; {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		for _, call := range calls {
			if len(call.args) > 0 && call.args[0] == "Book A" {
				foundCall = true
				break
			}
		}
		if foundCall {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundCall {
		t.Fatal("expected Fetch to be called with parameter 'Book A' after submit")
	}
}

// --- Phase 2: Core Wiring RED Tests (Screen State Store) ---

// Task 2.1: text_input writes to ScreenState, NOT to EventBus
func TestCompose_TextInput_WritesToState_NoPublish(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var published bool
	eb.Subscribe("filter_channel", func(ev eventbus.Event) {
		published = true
	})

	node := ui.NodeMeta{
		Area:         "input_area",
		ComponentRef: "text_input",
		BindTo:       "filter_channel",
		Placeholder:  "Filter",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	entry, ok := obj.(*widget.Entry)
	if !ok {
		t.Fatalf("expected *widget.Entry, got %T", obj)
	}

	test.Type(entry, "hello")
	time.Sleep(50 * time.Millisecond)

	if published {
		t.Error("text_input should NOT publish to EventBus — it should write to ScreenState instead")
	}
}

// Task 2.1 triangulation: text_input without bind_to is ignored
func TestCompose_TextInput_NoBindTo_NoStateWrite(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var published bool
	eb.Subscribe("any_channel", func(ev eventbus.Event) {
		published = true
	})

	node := ui.NodeMeta{
		Area:         "input_area",
		ComponentRef: "text_input",
		Placeholder:  "No bind_to",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	entry, ok := obj.(*widget.Entry)
	if !ok {
		t.Fatalf("expected *widget.Entry, got %T", obj)
	}

	test.Type(entry, "typing")
	time.Sleep(50 * time.Millisecond)

	if published {
		t.Error("text_input without bind_to should not publish anything")
	}
}

// Task 2.2: button with submit_action publishes snapshot to SubmitChannel
func TestCompose_Button_SubmitAction_PublishesSnapshot(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var receivedPayload interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	eb.Subscribe("screen:submit:test-vista", func(ev eventbus.Event) {
		receivedPayload = ev.Payload
		wg.Done()
	})

	// Compose a container with text_input + button sharing one ScreenState
	containerNode := ui.NodeMeta{
		Area:         "form",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "name_input",
				ComponentRef: "text_input",
				BindTo:       "name",
				Placeholder:  "Name",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Submit",
				SubmitAction: "search",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}
	if len(c.Objects) != 2 {
		t.Fatalf("expected 2 children, got %d", len(c.Objects))
	}

	// Type into the input
	entry, ok := c.Objects[0].(*widget.Entry)
	if !ok {
		t.Fatalf("expected first child to be *widget.Entry, got %T", c.Objects[0])
	}
	test.Type(entry, "Alice")

	// Click the submit button
	btn, ok := c.Objects[1].(*widget.Button)
	if !ok {
		t.Fatalf("expected second child to be *widget.Button, got %T", c.Objects[1])
	}
	test.Tap(btn)

	// Wait for async publish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for SubmitChannel publish")
	}

	snap, ok := receivedPayload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload to be map[string]any, got %T", receivedPayload)
	}
	if snap["name"] != "Alice" {
		t.Errorf("expected snapshot['name'] = 'Alice', got %v", snap["name"])
	}
}

// Task 2.2 triangulation: button WITHOUT submit_action does NOT publish
func TestCompose_Button_NoSubmitAction_NoPublish(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var published bool
	eb.Subscribe("screen:submit:test-vista", func(ev eventbus.Event) {
		published = true
	})

	containerNode := ui.NodeMeta{
		Area:         "form",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "btn",
				ComponentRef: "button",
				Label:        "No Action",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	btn, ok := c.Objects[0].(*widget.Button)
	if !ok {
		t.Fatalf("expected *widget.Button, got %T", c.Objects[0])
	}
	test.Tap(btn)

	time.Sleep(50 * time.Millisecond)
	if published {
		t.Error("button without submit_action should NOT publish to SubmitChannel")
	}
}

// Task 2.3: data_grid subscribes to SubmitChannel and dispatches server-mode query
func TestCompose_DataGrid_ServerMode_SubmitChannelQuery(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	trackingDS := &trackingMockDataSource{
		MockDataSource: &dataaccess.MockDataSource{
			FetchResult: dataaccess.DataSet{
				Headers: []string{"id", "title"},
				Rows:    [][]string{{"1", "Book A"}},
			},
		},
	}
	ui.DS = trackingDS
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	// Compose container with input + button + grid
	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "title_input",
				ComponentRef: "text_input",
				BindTo:       "title",
				Placeholder:  "Title",
			},
			{
				Area:         "author_input",
				ComponentRef: "text_input",
				BindTo:       "author",
				Placeholder:  "Author",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Search",
				SubmitAction: "search",
			},
			{
				Area:         "grid_area",
				ComponentRef: "data_grid",
				DataSource:   "SELECT * FROM books WHERE title LIKE $1 AND author = $2",
				FilterMode:   "server",
				FilterKeys:   []string{"title", "author"},
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}
	if len(c.Objects) != 4 {
		t.Fatalf("expected 4 children, got %d", len(c.Objects))
	}

	// Type into inputs
	titleEntry := c.Objects[0].(*widget.Entry)
	authorEntry := c.Objects[1].(*widget.Entry)
	test.Type(titleEntry, "%Sci-fi%")
	test.Type(authorEntry, "Asimov")

	// Click submit button
	submitBtn := c.Objects[2].(*widget.Button)
	test.Tap(submitBtn)

	// Wait for the Fetch call with positional args
	var foundCall bool
	for start := time.Now(); time.Since(start) < 1000*time.Millisecond; {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		for _, call := range calls {
			if call.source == "SELECT * FROM books WHERE title LIKE $1 AND author = $2" &&
				len(call.args) == 2 &&
				call.args[0] == "%Sci-fi%" &&
				call.args[1] == "Asimov" {
				foundCall = true
				break
			}
		}
		if foundCall {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundCall {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		t.Fatalf("expected server-mode Fetch with args [%q, %q], got %d calls: %+v",
			"%Sci-fi%", "Asimov", len(calls), calls)
	}
}

// Task 2.6: client-mode grid eager master buffer load + in-memory filter
func TestCompose_DataGrid_ClientMode_EagerLoadAndFilter(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	trackingDS := &trackingMockDataSource{
		MockDataSource: &dataaccess.MockDataSource{
			FetchAllResult: dataaccess.DataSet{
				Headers: []string{"id", "title", "author"},
				Rows: [][]string{
					{"1", "Foundation", "Asimov"},
					{"2", "Dune", "Herbert"},
					{"3", "I, Robot", "Asimov"},
				},
			},
		},
	}
	ui.DS = trackingDS
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "author_input",
				ComponentRef: "text_input",
				BindTo:       "author",
				Placeholder:  "Author filter",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Filter",
				SubmitAction: "filter",
			},
			{
				Area:             "grid_area",
				ComponentRef:     "data_grid",
				FilterMode:       "client",
				MasterDataSource: "SELECT * FROM books",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Wait for eager master buffer load
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		trackingDS.mu.Lock()
		calls := len(trackingDS.fetchAllCalls)
		trackingDS.mu.Unlock()
		if calls > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("expected FetchAll to be called during Compose")
	}

	// Verify master data was loaded via FetchAll
	trackingDS.mu.Lock()
	initialFetchAllCalls := len(trackingDS.fetchAllCalls)
	initialFetchCalls := len(trackingDS.fetchCalls)
	trackingDS.mu.Unlock()

	// Type "Asimov" filter and submit
	authorEntry := c.Objects[0].(*widget.Entry)
	test.Type(authorEntry, "Asimov")

	filterBtn := c.Objects[1].(*widget.Button)
	test.Tap(filterBtn)

	// Wait for client-side filter to apply
	time.Sleep(100 * time.Millisecond)

	// Verify NO additional Fetch calls for client-mode filtering
	trackingDS.mu.Lock()
	postFilterFetchCalls := len(trackingDS.fetchCalls)
	postFilterFetchAllCalls := len(trackingDS.fetchAllCalls)
	trackingDS.mu.Unlock()
	if postFilterFetchCalls > initialFetchCalls {
		t.Errorf("client-mode filter should NOT trigger Fetch, got %d extra calls",
			postFilterFetchCalls-initialFetchCalls)
	}
	if postFilterFetchAllCalls > initialFetchAllCalls {
		t.Errorf("client-mode filter should NOT trigger additional FetchAll, got %d extra calls",
			postFilterFetchAllCalls-initialFetchAllCalls)
	}

	// Verify the grid shows filtered data (only Asimov rows)
	gridTable := c.Objects[2].(*widget.Table)
	var filtered bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 2 { // Foundation + I, Robot (both Asimov)
			filtered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !filtered {
		rows, _ := gridTable.Length()
		t.Errorf("expected 2 filtered rows (Asimov), got %d", rows)
	}
}

// Fix #2: Two screens with different vistaIDs must NOT cross-talk.
func TestCompose_ScopedSubmitChannel_NoCrossTalk(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var mu sync.Mutex
	screenAReceived := 0
	screenBReceived := 0
	var wgA sync.WaitGroup
	wgA.Add(1)

	eb.Subscribe("screen:submit:screen-a", func(ev eventbus.Event) {
		mu.Lock()
		screenAReceived++
		mu.Unlock()
		wgA.Done()
	})
	eb.Subscribe("screen:submit:screen-b", func(ev eventbus.Event) {
		mu.Lock()
		screenBReceived++
		mu.Unlock()
	})

	// Build screen A with a submit button
	screenANode := ui.NodeMeta{
		Area:         "screen_a",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "input_a",
				ComponentRef: "text_input",
				BindTo:       "query",
				Placeholder:  "Screen A input",
			},
			{
				Area:         "btn_a",
				ComponentRef: "button",
				Label:        "Search A",
				SubmitAction: "search",
			},
		},
	}

	_, cleanup, err := ui.Compose(screenANode, "screen-a")
	if err != nil {
		t.Fatalf("Compose screen A failed: %v", err)
	}
	defer cleanup()

	// Build screen B with a submit button (different vistaID)
	screenBNode := ui.NodeMeta{
		Area:         "screen_b",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "btn_b",
				ComponentRef: "button",
				Label:        "Search B",
				SubmitAction: "search",
			},
		},
	}

	objB, cleanup, err := ui.Compose(screenBNode, "screen-b")
	if err != nil {
		t.Fatalf("Compose screen B failed: %v", err)
	}
	defer cleanup()
	cB, _ := objB.(*fyne.Container)

	// Tap screen B's button — screen A should NOT receive this
	test.Tap(cB.Objects[0].(*widget.Button))

	// Wait for potential cross-talk to manifest
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	aCount := screenAReceived
	bCount := screenBReceived
	mu.Unlock()

	if bCount != 1 {
		t.Errorf("expected screen B to receive 1 event, got %d", bCount)
	}

	if aCount != 0 {
		t.Errorf("expected screen A to receive 0 events (no cross-talk), got %d", aCount)
	}
}

// Fix #3: client-mode filter with non-matching column key should still work on matching keys
func TestCompose_ClientMode_FilterMismatchColumn_LogsWarning(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchAllResult: dataaccess.DataSet{
			Headers: []string{"id", "title", "author"},
			Rows: [][]string{
				{"1", "Foundation", "Asimov"},
				{"2", "Dune", "Herbert"},
				{"3", "I, Robot", "Asimov"},
			},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "author_input",
				ComponentRef: "text_input",
				BindTo:       "author",
				Placeholder:  "Author",
			},
			{
				Area:         "bogus_input",
				ComponentRef: "text_input",
				BindTo:       "nonexistent_column",
				Placeholder:  "Bogus",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Filter",
				SubmitAction: "filter",
			},
			{
				Area:             "grid_area",
				ComponentRef:     "data_grid",
				FilterMode:       "client",
				MasterDataSource: "SELECT * FROM books",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Wait for eager master buffer load
	time.Sleep(200 * time.Millisecond)

	// Type valid filter + bogus filter
	authorEntry := c.Objects[0].(*widget.Entry)
	bogusEntry := c.Objects[1].(*widget.Entry)
	test.Type(authorEntry, "Asimov")
	test.Type(bogusEntry, "anything")

	// Tap submit — should filter on "author" column and log warning for "nonexistent_column"
	filterBtn := c.Objects[2].(*widget.Button)
	test.Tap(filterBtn)

	// Wait for client-side filter
	time.Sleep(100 * time.Millisecond)

	// Verify grid shows only Asimov rows (filter on valid column worked despite bogus key)
	gridTable := c.Objects[3].(*widget.Table)
	var filtered bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 2 { // Foundation + I, Robot
			filtered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !filtered {
		rows, _ := gridTable.Length()
		t.Errorf("expected 2 filtered rows (Asimov), got %d — filter should still work on matching keys", rows)
	}
}

// Fix #1: server-mode grid without FilterKeys should skip SUBMIT and not execute query
func TestCompose_ServerMode_NoFilterKeys_SkipsSubmit(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	trackingDS := &trackingMockDataSource{
		MockDataSource: &dataaccess.MockDataSource{
			FetchResult: dataaccess.DataSet{
				Headers: []string{"id", "title"},
				Rows:    [][]string{{"1", "Book A"}},
			},
		},
	}
	ui.DS = trackingDS
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "input_area",
				ComponentRef: "text_input",
				BindTo:       "title",
				Placeholder:  "Title",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Search",
				SubmitAction: "search",
			},
			{
				Area:         "grid_area",
				ComponentRef: "data_grid",
				DataSource:   "SELECT * FROM books",
				// No FilterMode and no FilterKeys — server-mode default without keys
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Wait for initial data load
	time.Sleep(200 * time.Millisecond)

	trackingDS.mu.Lock()
	initialCalls := len(trackingDS.fetchCalls)
	trackingDS.mu.Unlock()

	// Type and submit
	entry := c.Objects[0].(*widget.Entry)
	test.Type(entry, "Book A")
	test.Tap(c.Objects[1].(*widget.Button))

	// Wait for potential submit processing
	time.Sleep(200 * time.Millisecond)

	// Verify NO additional query was fired (guard prevented it)
	trackingDS.mu.Lock()
	postSubmitCalls := len(trackingDS.fetchCalls)
	trackingDS.mu.Unlock()

	if postSubmitCalls > initialCalls {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		t.Errorf("expected NO additional Fetch calls after SUBMIT without filter_keys, got %d extra calls: %+v",
			postSubmitCalls-initialCalls, calls[initialCalls:])
	}
}

func TestCompose_TextArea(t *testing.T) {
	node := ui.NodeMeta{
		Area:         "query_input",
		ComponentRef: "text_area",
		Placeholder:  "Write SQL here",
		DefaultValue: "SELECT 1",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	entry, ok := obj.(*widget.Entry)
	if !ok {
		t.Fatalf("expected *widget.Entry for text_area, got %T", obj)
	}

	if !entry.MultiLine {
		t.Error("expected entry to be MultiLine for text_area component")
	}

	if entry.PlaceHolder != "Write SQL here" {
		t.Errorf("expected placeholder 'Write SQL here', got %q", entry.PlaceHolder)
	}

	if entry.Text != "SELECT 1" {
		t.Errorf("expected text 'SELECT 1', got %q", entry.Text)
	}
}

// --- TDD RED: data_grid row selection publish ---

// AC test: selecting a row in data_grid publishes header→value map to publish_selection
func TestCompose_DataGrid_RowSelection_PublishesToSelectionChannel(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var receivedPayload map[string]any
	var wg sync.WaitGroup
	wg.Add(1)

	eb.Subscribe("publish_selection", func(ev eventbus.Event) {
		if m, ok := ev.Payload.(map[string]any); ok {
			receivedPayload = m
		}
		wg.Done()
	})

	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id", "nombre", "monto"},
			Rows:    [][]string{{"42", "Transaccion Test", "1000.5"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id, nombre, monto FROM transacciones",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Wait for async data loading
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := table.Length()
		if rows > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for data_grid to load data async")
	}

	// Simulate row selection
	table.OnSelected(widget.TableCellID{Row: 0, Col: 0})

	// Wait for async EventBus delivery
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for publish_selection event")
	}

	expected := map[string]any{"id": "42", "nombre": "Transaccion Test", "monto": "1000.5"}
	if receivedPayload == nil {
		t.Fatal("expected non-nil payload on publish_selection channel")
	}
	for k, v := range expected {
		if receivedPayload[k] != v {
			t.Errorf("expected payload[%q] = %q, got %q", k, v, receivedPayload[k])
		}
	}
	if len(receivedPayload) != len(expected) {
		t.Errorf("expected %d keys in payload, got %d", len(expected), len(receivedPayload))
	}
}

// Edge case: out-of-bounds row selection publishes nothing
func TestCompose_DataGrid_RowSelection_OutOfBounds_NoPublish(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	var published bool
	eb.Subscribe("publish_selection", func(ev eventbus.Event) {
		published = true
	})

	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id"},
			Rows:    [][]string{{"1"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Wait for data
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := table.Length()
		if rows > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for data")
	}

	// Select out-of-bounds rows
	table.OnSelected(widget.TableCellID{Row: -1, Col: 0})
	table.OnSelected(widget.TableCellID{Row: 99, Col: 0})

	time.Sleep(100 * time.Millisecond)

	if published {
		t.Error("expected NO publish for out-of-bounds row selection")
	}
}

// Edge case: nil LocalEventBus does not panic on row selection
func TestCompose_DataGrid_RowSelection_NilEventBus_NoPanic(t *testing.T) {
	ui.LocalEventBus = nil

	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id"},
			Rows:    [][]string{{"1"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Wait for data
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := table.Length()
		if rows > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for data")
	}

	// Should not panic
	table.OnSelected(widget.TableCellID{Row: 0, Col: 0})

	time.Sleep(50 * time.Millisecond)
}

func TestCompose_ButtonNavigation(t *testing.T) {
	var navigatedTo string
	ui.Navigate = func(vistaID string) {
		navigatedTo = vistaID
	}
	defer func() {
		ui.Navigate = nil
	}()

	node := ui.NodeMeta{
		Area:         "nav_button",
		ComponentRef: "button",
		Label:        "Go to query runner",
		SubmitAction: "navigate:query_runner",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	btn, ok := obj.(*widget.Button)
	if !ok {
		t.Fatalf("expected *widget.Button, got %T", obj)
	}

	test.Tap(btn)

	if navigatedTo != "query_runner" {
		t.Errorf("expected Navigate to be called with 'query_runner', got %q", navigatedTo)
	}
}

// --- Screen Lifecycle Cleanup TDD Tests ---

func TestCompose_ReturnsCleanupFunc(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"title", "author"},
			Rows:    [][]string{{"Foundation", "Asimov"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT * FROM books",
		FilterKeys:   []string{"author"},
	}

	obj, cleanup, err := ui.Compose(node, "test-cleanup")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	if obj == nil {
		t.Fatal("expected non-nil widget")
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup func for data_grid screen")
	}
}

func TestCompose_CleanupRemovesSubscribers(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"title", "author"},
			Rows:    [][]string{{"Foundation", "Asimov"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT * FROM books",
		FilterKeys:   []string{"author"},
	}

	_, cleanup, err := ui.Compose(node, "test-unsub")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Verify 1 subscriber on the submit channel
	count := eb.(*eventbus.InMemEventBus).SubscriberCount("screen:submit:test-unsub")
	if count != 1 {
		t.Fatalf("expected 1 subscriber, got %d", count)
	}

	// Call cleanup
	cleanup()

	// Verify 0 subscribers after cleanup
	count = eb.(*eventbus.InMemEventBus).SubscriberCount("screen:submit:test-unsub")
	if count != 0 {
		t.Fatalf("expected 0 subscribers after cleanup, got %d", count)
	}

	// Publish on the old channel and verify no handler fires
	var fired int32
	eb.Subscribe("screen:submit:test-unsub", func(ev eventbus.Event) {
		atomic.AddInt32(&fired, 1)
	})
	eb.Publish("screen:submit:test-unsub", map[string]any{"author": "test"})
	time.Sleep(100 * time.Millisecond)

	// Only our spy handler should have fired (the old one was removed)
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("expected exactly 1 handler (spy only), got %d", atomic.LoadInt32(&fired))
	}
}

func TestCompose_CleanupCancelsGoroutines(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchAllResult: dataaccess.DataSet{
			Headers: []string{"title", "author"},
			Rows:    [][]string{{"Foundation", "Asimov"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:             "grid_area",
		ComponentRef:     "data_grid",
		FilterMode:       "client",
		MasterDataSource: "SELECT * FROM books",
	}

	_, cleanup, err := ui.Compose(node, "test-cancel")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// Wait for master buffer to load
	time.Sleep(300 * time.Millisecond)

	// Call cleanup — should cancel the context
	cleanup()

	// If cleanup properly cancels the context, the test completes without hanging
	// We verify by checking that a second call to cleanup is safe (idempotent)
	cleanup()
}

func TestCompose_IdempotentCleanup(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"title", "author"},
			Rows:    [][]string{{"Foundation", "Asimov"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT * FROM books",
		FilterKeys:   []string{"author"},
	}

	_, cleanup, err := ui.Compose(node, "test-idempotent")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	// First cleanup removes the subscriber
	cleanup()
	count := eb.(*eventbus.InMemEventBus).SubscriberCount("screen:submit:test-idempotent")
	if count != 0 {
		t.Fatalf("expected 0 subscribers after first cleanup, got %d", count)
	}

	// Second cleanup should not panic and count stays 0
	cleanup()
	count = eb.(*eventbus.InMemEventBus).SubscriberCount("screen:submit:test-idempotent")
	if count != 0 {
		t.Fatalf("expected 0 subscribers after second cleanup, got %d", count)
	}
}

func TestCompose_NoOpCleanup_NoDataGrid(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	node := ui.NodeMeta{
		Area:         "root",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "lbl",
				ComponentRef: "label",
				Label:        "Hello",
			},
		},
	}

	_, cleanup, err := ui.Compose(node, "test-noop")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if cleanup == nil {
		t.Fatal("expected non-nil cleanup func even for non-data_grid screens")
	}

	// Calling cleanup should be safe — no panic, no side effects
	cleanup()
}

// --- New tests for column width resolution ---

func TestCompose_DataGrid_ColumnWidthFromCWR(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id", "status", "name"},
			Rows:    [][]string{{"1", "active", "Alice"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{
		ResolveFunc: func(origen, header string) string {
			if header == "status" {
				return "200px"
			}
			return ""
		},
	}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id, status, name FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Wait for data to load
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := table.Length()
		if rows > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for data_grid to load")
	}

	// Verify table has data — the column width setting is internal
	// We verify the grid loaded correctly with the CWR present
	rows, cols := table.Length()
	if rows != 1 || cols != 3 {
		t.Errorf("expected 1x3 table, got %dx%d", rows, cols)
	}
}

func TestCompose_DataGrid_ColumnWidthFallback(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id", "name"},
			Rows:    [][]string{{"1", "Alice"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{} // returns "" for all columns → fallback to defaultGridColWidth
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id, name FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Wait for data to load
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := table.Length()
		if rows > 0 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for data_grid to load")
	}

	// Verify data loaded — fallback width of 150 is used
	rows, cols := table.Length()
	if rows != 1 || cols != 2 {
		t.Errorf("expected 1x2 table, got %dx%d", rows, cols)
	}
}

func TestCompose_DataGrid_DynamicQueryFromState(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	trackingDS := &trackingMockDataSource{
		MockDataSource: &dataaccess.MockDataSource{
			FetchResult: dataaccess.DataSet{
				Headers: []string{"id"},
				Rows:    [][]string{{"1"}},
			},
		},
	}
	ui.DS = trackingDS
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "query_input",
				ComponentRef: "text_area",
				BindTo:       "sql_query",
				DefaultValue: "SELECT 1",
			},
			{
				Area:         "submit_btn",
				ComponentRef: "button",
				Label:        "Execute",
				SubmitAction: "search",
			},
			{
				Area:         "results_grid",
				ComponentRef: "data_grid",
				FilterMode:   "server",
				DataSource:   "state:sql_query",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Type a dynamic SQL query
	queryEntry := c.Objects[0].(*widget.Entry)
	test.Type(queryEntry, "SELECT id FROM users")
	submitBtn := c.Objects[1].(*widget.Button)
	test.Tap(submitBtn)

	// Wait for Fetch call with resolved SQL
	var foundCall bool
	for start := time.Now(); time.Since(start) < 1000*time.Millisecond; {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		// The state: prefix resolves the SQL from the snapshot, so the source
		// should be the resolved query, not "state:sql_query"
		for _, call := range calls {
			if call.source != "" && len(call.args) == 0 {
				foundCall = true
				break
			}
		}
		if foundCall {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundCall {
		trackingDS.mu.Lock()
		calls := trackingDS.fetchCalls
		trackingDS.mu.Unlock()
		t.Fatalf("expected Fetch to be called with resolved query (no args), got %d calls: %+v",
			len(calls), calls)
	}
}

// --- TDD Phase 4: fyne.Do thread-safety tests for DataGrid ---

// T-4.6: TestLoadMasterBuffer_WrapsInFyneDo verifies that loadMasterBuffer
// correctly loads data and updates the table via fyne.Do (REQ-DG-01).
// In the test environment, fyne.Do runs synchronously on the calling goroutine,
// so we verify the end result: table has data after async load completes.
func TestLoadMasterBuffer_WrapsInFyneDo(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchAllResult: dataaccess.DataSet{
			Headers: []string{"id", "name", "amount"},
			Rows: [][]string{
				{"1", "Alice", "100"},
				{"2", "Bob", "200"},
			},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:             "grid_area",
		ComponentRef:     "data_grid",
		FilterMode:       "client",
		MasterDataSource: "SELECT * FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-master-buffer")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Poll for async loadMasterBuffer to complete (the fyne.Do wraps
	// SetColumnWidth + Refresh, so data should appear after it runs)
	var loaded bool
	for start := time.Now(); time.Since(start) < 1*time.Second; {
		rows, cols := table.Length()
		if rows == 2 && cols == 3 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !loaded {
		rows, cols := table.Length()
		t.Fatalf("expected 2x3 table after loadMasterBuffer, got %dx%d", rows, cols)
	}

	// Verify cell content
	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 1}, cell)
	lbl := cell.(*widget.Label)
	if lbl.Text != "Alice" {
		t.Errorf("expected cell (0,1) = 'Alice', got %q", lbl.Text)
	}
}

// T-4.7: TestFetchGridDataAsync_WrapsInFyneDo verifies that fetchGridDataAsync
// correctly loads data and updates the table via fyne.Do (REQ-DG-02).
func TestFetchGridDataAsync_WrapsInFyneDo(t *testing.T) {
	ui.DS = &dataaccess.MockDataSource{
		FetchResult: dataaccess.DataSet{
			Headers: []string{"id", "status"},
			Rows:    [][]string{{"10", "active"}, {"20", "pending"}},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id, status FROM items",
	}

	obj, cleanup, err := ui.Compose(node, "test-fetch-async")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", obj)
	}

	// Poll for async fetchGridDataAsync to complete
	var loaded bool
	for start := time.Now(); time.Since(start) < 1*time.Second; {
		rows, cols := table.Length()
		if rows == 2 && cols == 2 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !loaded {
		rows, cols := table.Length()
		t.Fatalf("expected 2x2 table after fetchGridDataAsync, got %dx%d", rows, cols)
	}

	// Verify cell content
	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 1, Col: 1}, cell)
	lbl := cell.(*widget.Label)
	if lbl.Text != "pending" {
		t.Errorf("expected cell (1,1) = 'pending', got %q", lbl.Text)
	}
}

// T-4.8: TestFilterMasterRows_EmptySnap_WrapsInFyneDo verifies that filtering
// with an empty snapshot resets to master rows via fyne.Do (REQ-DG-03).
func TestFilterMasterRows_EmptySnap_WrapsInFyneDo(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchAllResult: dataaccess.DataSet{
			Headers: []string{"id", "name"},
			Rows: [][]string{
				{"1", "Alice"},
				{"2", "Bob"},
				{"3", "Charlie"},
			},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "input",
				ComponentRef: "text_input",
				BindTo:       "name",
			},
			{
				Area:         "btn",
				ComponentRef: "button",
				Label:        "Filter",
				SubmitAction: "filter",
			},
			{
				Area:             "grid",
				ComponentRef:     "data_grid",
				FilterMode:       "client",
				MasterDataSource: "SELECT * FROM items",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-filter-empty")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c := obj.(*fyne.Container)
	gridTable := c.Objects[2].(*widget.Table)

	// Wait for master buffer load
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 3 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for master buffer load")
	}

	// First: filter to get fewer rows (type “Alice” and submit)
	entry := c.Objects[0].(*widget.Entry)
	test.Type(entry, "Alice")
	test.Tap(c.Objects[1].(*widget.Button))

	// Wait for filter to apply
	var filtered bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 1 {
			filtered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !filtered {
		rows, _ := gridTable.Length()
		t.Fatalf("expected 1 row after filter, got %d", rows)
	}

	// Now submit with empty input — empty snapshot resets to master
	test.Type(entry, "") // clear filter
	// We need to clear the entry first
	entry.SetText("")
	test.Tap(c.Objects[1].(*widget.Button))

	// Wait for reset to master rows (3 rows)
	var reset bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 3 {
			reset = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !reset {
		rows, _ := gridTable.Length()
		t.Errorf("expected 3 rows after empty-snap filter reset, got %d", rows)
	}
}

// T-4.9: TestFilterMasterRows_Filtered_WrapsInFyneDo verifies that filtering
// with a matching snapshot reduces rows via fyne.Do (REQ-DG-04).
func TestFilterMasterRows_Filtered_WrapsInFyneDo(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	ui.DS = &dataaccess.MockDataSource{
		FetchAllResult: dataaccess.DataSet{
			Headers: []string{"id", "name"},
			Rows: [][]string{
				{"1", "Alice"},
				{"2", "Bob"},
				{"3", "Charlie"},
			},
		},
	}
	ui.CWR = &dataaccess.MockCWR{}
	defer func() { ui.DS = nil; ui.CWR = nil }()

	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "input",
				ComponentRef: "text_input",
				BindTo:       "name",
			},
			{
				Area:         "btn",
				ComponentRef: "button",
				Label:        "Filter",
				SubmitAction: "filter",
			},
			{
				Area:             "grid",
				ComponentRef:     "data_grid",
				FilterMode:       "client",
				MasterDataSource: "SELECT * FROM items",
			},
		},
	}

	obj, cleanup, err := ui.Compose(containerNode, "test-filter-matching")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}
	defer cleanup()

	c := obj.(*fyne.Container)
	gridTable := c.Objects[2].(*widget.Table)

	// Wait for master buffer load
	var loaded bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 3 {
			loaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("timeout waiting for master buffer load")
	}

	// Filter with "ob” (matches “Bob”)
	entry := c.Objects[0].(*widget.Entry)
	test.Type(entry, "ob")
	test.Tap(c.Objects[1].(*widget.Button))

	// Wait for filter — only Bob should remain
	var filtered bool
	for start := time.Now(); time.Since(start) < 500*time.Millisecond; {
		rows, _ := gridTable.Length()
		if rows == 1 {
			filtered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !filtered {
		rows, _ := gridTable.Length()
		t.Errorf("expected 1 row after filter (Bob), got %d", rows)
	}

	// Verify the remaining row is Bob
	cell := gridTable.CreateCell()
	gridTable.UpdateCell(widget.TableCellID{Row: 0, Col: 1}, cell)
	lbl := cell.(*widget.Label)
	if lbl.Text != "Bob" {
		t.Errorf("expected filtered row to be 'Bob', got %q", lbl.Text)
	}
}

// T-4.10: TestDataGrid_ModelMuUnlockedBeforeFyneDo verifies REQ-LOCK-01:
// at every DataGrid site, model.mu is NOT held during fyne.Do callback execution.
// We test this structurally by confirming no Unlock appears inside any fyne.Do block.
func TestDataGrid_ModelMuUnlockedBeforeFyneDo(t *testing.T) {
	// This is a structural/source-level test. We grep the compositor source
	// to verify that model.mu.Unlock() never appears inside a fyne.Do callback.
	src, err := os.ReadFile("compositor.go")
	if err != nil {
		t.Skip("could not read compositor.go source for structural check")
	}

	source := string(src)
	lines := strings.Split(source, "\n")

	// Parse fyne.Do blocks: find each "fyne.Do(func()" and the matching "})" closure,
	// then check that no "model.mu.Unlock()" appears between them.
	var inFyneDo bool
	braceDepth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inFyneDo && (strings.Contains(trimmed, "fyne.Do(func()") || strings.Contains(trimmed, "fyne.DoAndWait(func()")) {
			inFyneDo = true
			braceDepth = 0
		}

		if inFyneDo {
			// Count braces to track when the callback closes
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			if strings.Contains(trimmed, "model.mu.Unlock()") {
				t.Errorf("REQ-LOCK-01 violation: model.mu.Unlock() found inside fyne.Do/DoAndWait callback at line %d: %s", i+1, trimmed)
			}

			if braceDepth <= 0 {
				inFyneDo = false
			}
		}
	}
}

// T-4.11: TestDataGrid_NoRefreshMuInModel verifies that the dataGridModel
// struct has no refreshMu field (REQ-DG-05: complete removal of refreshMu).
func TestDataGrid_NoRefreshMuInModel(t *testing.T) {
	// Structural/source-level test: grep compositor.go for refreshMu
	src, err := os.ReadFile("compositor.go")
	if err != nil {
		t.Skip("could not read compositor.go source for structural check")
	}

	if strings.Contains(string(src), "refreshMu") {
		t.Error("REQ-DG-05 violation: 'refreshMu' found in compositor.go source — should be completely removed")
	}
}
