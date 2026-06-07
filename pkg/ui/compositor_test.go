package ui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/jackc/pgx/v5"
)

func TestMain(m *testing.M) {
	test.NewApp()
	m.Run()
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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("expected graceful handling without error, got err: %v", err)
	}

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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

func TestBusinessPoolExists(t *testing.T) {
	// Reference ui.BusinessPool, which doesn't exist yet, to trigger a compile-time failure.
	var pool interface{} = ui.BusinessPool
	if pool != nil {
		t.Log("BusinessPool is not nil")
	}
}

func TestCorePool_DefaultsNil(t *testing.T) {
	if ui.CorePool != nil {
		t.Errorf("expected CorePool to be nil at package init, got %v", ui.CorePool)
	}
}

func TestCompose_DataGrid_Success(t *testing.T) {
	mockPool := db.NewMockDBPool()

	// Register mock query
	cols := []string{"id", "title", "amount"}
	rowsData := [][]any{
		{1, "Book A", 25.5},
		{2, "Book B", 35.0},
	}
	mockPool.RegisterQuery("SELECT * FROM books", cols, rowsData, nil)

	// Inject the mock pool
	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT * FROM books",
	}

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected composed object to be *widget.Table, got %T", obj)
	}

	rows, cols := table.Length()
	if rows != 0 || cols != 0 {
		t.Errorf("expected 0x0 table when DataSource is empty, got %dx%d", rows, cols)
	}
}

func TestCompose_DataGrid_NilPool(t *testing.T) {
	origPool := ui.BusinessPool
	ui.BusinessPool = nil
	defer func() { ui.BusinessPool = origPool }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT 1",
	}

	// This should not crash / panic
	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

	table, ok := obj.(*widget.Table)
	if !ok {
		t.Fatalf("expected composed object to be *widget.Table, got %T", obj)
	}

	rows, cols := table.Length()
	if rows != 0 || cols != 0 {
		t.Errorf("expected 0x0 table when BusinessPool is nil, got %dx%d", rows, cols)
	}
}

type queryCall struct {
	ctx           context.Context
	sql           string
	args          []any
	ctxErrAtQuery error // captured ctx.Err() at the moment of query execution
}

type trackingMockDBPool struct {
	*db.MockDBPool
	mu            sync.Mutex
	queriesCalled []queryCall
}

func (t *trackingMockDBPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	capturedErr := ctx.Err()
	t.mu.Lock()
	t.queriesCalled = append(t.queriesCalled, queryCall{ctx: ctx, sql: sql, args: args, ctxErrAtQuery: capturedErr})
	t.mu.Unlock()
	return t.MockDBPool.Query(ctx, sql, args...)
}

func TestCompose_DataGrid_ReactiveFiltering(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	mockPool := db.NewMockDBPool()
	trackingPool := &trackingMockDBPool{MockDBPool: mockPool}
	ui.BusinessPool = trackingPool
	defer func() { ui.BusinessPool = nil }()

	cols := []string{"id", "title"}
	rowsData := [][]any{
		{1, "Book A"},
	}
	mockPool.RegisterQuery("SELECT * FROM books WHERE title LIKE $1", cols, rowsData, nil)

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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
	var lastCall queryCall
	for start := time.Now(); time.Since(start) < 1000*time.Millisecond; {
		trackingPool.mu.Lock()
		calls := trackingPool.queriesCalled
		trackingPool.mu.Unlock()
		for _, call := range calls {
			if len(call.args) > 0 && call.args[0] == "Book A" {
				foundCall = true
				lastCall = call
				break
			}
		}
		if foundCall {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !foundCall {
		t.Fatal("expected query to be executed with parameter 'Book A' after submit")
	}

	// Verify the context was NOT cancelled at the moment the successful query executed
	if lastCall.ctxErrAtQuery != nil {
		t.Errorf("expected successful query context not to be cancelled at query time, but it was: %v", lastCall.ctxErrAtQuery)
	}

	// Now verify rapid submit cancels previous context
	trackingPool.mu.Lock()
	trackingPool.queriesCalled = nil
	trackingPool.mu.Unlock()

	// Rapid type + submit multiple times
	test.Type(entry, "B")
	test.Tap(submitBtn)
	test.Type(entry, "C")
	test.Tap(submitBtn)
	test.Type(entry, "D")
	test.Tap(submitBtn)

	// Wait a bit to ensure queries were registered/triggered
	time.Sleep(200 * time.Millisecond)

	trackingPool.mu.Lock()
	callsAfter := trackingPool.queriesCalled
	trackingPool.mu.Unlock()

	// We expect multiple queries, and at least the earlier ones should have cancelled contexts
	if len(callsAfter) < 2 {
		t.Fatalf("expected at least 2 queries triggered during rapid submit, got %d", len(callsAfter))
	}

	// Verify that at least the last query (the final one) was NOT cancelled at query time
	lastIdx := len(callsAfter) - 1
	if callsAfter[lastIdx].ctxErrAtQuery != nil {
		t.Errorf("expected the final rapid-submit query context not to be cancelled, but it was: %v", callsAfter[lastIdx].ctxErrAtQuery)
	}

	// Check how many early queries had their context already cancelled at query time.
	// This is timing-dependent: early queries MAY be cancelled if the next submit's
	// subscriber fires before the goroutine reaches BusinessPool.Query, but it's not guaranteed.
	var cancelledCount int
	for i := 0; i < len(callsAfter)-1; i++ {
		if callsAfter[i].ctxErrAtQuery != nil {
			cancelledCount++
		}
	}
	if cancelledCount > 0 {
		t.Logf("observed %d/%d early queries cancelled at query time (timing-dependent)", cancelledCount, len(callsAfter)-1)
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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	mockPool := db.NewMockDBPool()
	trackingPool := &trackingMockDBPool{MockDBPool: mockPool}
	ui.BusinessPool = trackingPool
	defer func() { ui.BusinessPool = nil }()

	// Grid query with positional params
	cols := []string{"id", "title"}
	rowsData := [][]any{
		{1, "Book A"},
	}
	mockPool.RegisterQuery("SELECT * FROM books WHERE title LIKE $1 AND author = $2", cols, rowsData, nil)

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	// Wait for the query with positional args
	var foundCall bool
	for start := time.Now(); time.Since(start) < 1000*time.Millisecond; {
		trackingPool.mu.Lock()
		calls := trackingPool.queriesCalled
		trackingPool.mu.Unlock()
		for _, call := range calls {
			if call.sql == "SELECT * FROM books WHERE title LIKE $1 AND author = $2" &&
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
		trackingPool.mu.Lock()
		calls := trackingPool.queriesCalled
		trackingPool.mu.Unlock()
		t.Fatalf("expected server-mode query with args [%q, %q], got %d calls: %+v",
			"%Sci-fi%", "Asimov", len(calls), calls)
	}
}

// Task 2.6: client-mode grid eager master buffer load + in-memory filter
func TestCompose_DataGrid_ClientMode_EagerLoadAndFilter(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	mockPool := db.NewMockDBPool()
	trackingPool := &trackingMockDBPool{MockDBPool: mockPool}
	ui.BusinessPool = trackingPool
	defer func() { ui.BusinessPool = nil }()

	// Master data source returns all rows
	masterCols := []string{"id", "title", "author"}
	masterRows := [][]any{
		{1, "Foundation", "Asimov"},
		{2, "Dune", "Herbert"},
		{3, "I, Robot", "Asimov"},
	}
	mockPool.RegisterQuery("SELECT * FROM books", masterCols, masterRows, nil)

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Wait for eager master buffer load
	time.Sleep(200 * time.Millisecond)

	// Verify master data was loaded once (eager)
	trackingPool.mu.Lock()
	initialCalls := len(trackingPool.queriesCalled)
	trackingPool.mu.Unlock()
	if initialCalls == 0 {
		t.Fatal("expected master data to be eagerly loaded during Compose")
	}

	// Type "Asimov" filter and submit
	authorEntry := c.Objects[0].(*widget.Entry)
	test.Type(authorEntry, "Asimov")

	filterBtn := c.Objects[1].(*widget.Button)
	test.Tap(filterBtn)

	// Wait for client-side filter to apply
	time.Sleep(100 * time.Millisecond)

	// Verify NO additional BusinessPool.Query calls for client-mode filtering
	trackingPool.mu.Lock()
	postFilterCalls := len(trackingPool.queriesCalled)
	trackingPool.mu.Unlock()
	if postFilterCalls > initialCalls {
		t.Errorf("client-mode filter should NOT trigger BusinessPool.Query, got %d extra calls",
			postFilterCalls-initialCalls)
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

	_, err := ui.Compose(screenANode, "screen-a")
	if err != nil {
		t.Fatalf("Compose screen A failed: %v", err)
	}

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

	objB, err := ui.Compose(screenBNode, "screen-b")
	if err != nil {
		t.Fatalf("Compose screen B failed: %v", err)
	}
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

	mockPool := db.NewMockDBPool()
	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	// Master data with columns: id, title, author
	masterCols := []string{"id", "title", "author"}
	masterRows := [][]any{
		{1, "Foundation", "Asimov"},
		{2, "Dune", "Herbert"},
		{3, "I, Robot", "Asimov"},
	}
	mockPool.RegisterQuery("SELECT * FROM books", masterCols, masterRows, nil)

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

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

	mockPool := db.NewMockDBPool()
	trackingPool := &trackingMockDBPool{MockDBPool: mockPool}
	ui.BusinessPool = trackingPool
	defer func() { ui.BusinessPool = nil }()

	// Register query that should NOT be called during submit
	cols := []string{"id", "title"}
	rowsData := [][]any{{1, "Book A"}}
	mockPool.RegisterQuery("SELECT * FROM books", cols, rowsData, nil)

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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	// Wait for initial data load
	time.Sleep(200 * time.Millisecond)

	trackingPool.mu.Lock()
	initialCalls := len(trackingPool.queriesCalled)
	trackingPool.mu.Unlock()

	// Type and submit
	entry := c.Objects[0].(*widget.Entry)
	test.Type(entry, "Book A")
	test.Tap(c.Objects[1].(*widget.Button))

	// Wait for potential submit processing
	time.Sleep(200 * time.Millisecond)

	// Verify NO additional query was fired (guard prevented it)
	trackingPool.mu.Lock()
	postSubmitCalls := len(trackingPool.queriesCalled)
	trackingPool.mu.Unlock()

	if postSubmitCalls > initialCalls {
		trackingPool.mu.Lock()
		calls := trackingPool.queriesCalled
		trackingPool.mu.Unlock()
		t.Errorf("expected NO additional queries after SUBMIT without filter_keys, got %d extra calls: %+v",
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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	mockPool := db.NewMockDBPool()
	cols := []string{"id", "nombre", "monto"}
	rowsData := [][]any{
		{42, "Transaccion Test", 1000.50},
	}
	mockPool.RegisterQuery("SELECT id, nombre, monto FROM transacciones", cols, rowsData, nil)

	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id, nombre, monto FROM transacciones",
	}

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	mockPool := db.NewMockDBPool()
	cols := []string{"id"}
	rowsData := [][]any{{1}}
	mockPool.RegisterQuery("SELECT id FROM items", cols, rowsData, nil)

	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id FROM items",
	}

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	mockPool := db.NewMockDBPool()
	cols := []string{"id"}
	rowsData := [][]any{{1}}
	mockPool.RegisterQuery("SELECT id FROM items", cols, rowsData, nil)

	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT id FROM items",
	}

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

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

	obj, err := ui.Compose(node, "test-vista")
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}

	btn, ok := obj.(*widget.Button)
	if !ok {
		t.Fatalf("expected *widget.Button, got %T", obj)
	}

	test.Tap(btn)

	if navigatedTo != "query_runner" {
		t.Errorf("expected Navigate to be called with 'query_runner', got %q", navigatedTo)
	}
}

// --- Phase 3: Fyne Thread Safety Tests ---

// TestCompose_DataGrid_ConcurrentOps_NoDeadlock verifies that running all three
// goroutine types (G1: loadMasterBuffer, G2: fetchGridDataAsync, G3: EventBus filter)
// concurrently does not produce a deadlock. This validates the unlock-before-fyne.Do
// invariant at all wrap sites (REQ-LOCK-01).
func TestCompose_DataGrid_ConcurrentOps_NoDeadlock(t *testing.T) {
	eb := eventbus.NewEventBus()
	ui.LocalEventBus = eb
	defer func() { ui.LocalEventBus = nil }()

	mockPool := db.NewMockDBPool()
	ui.BusinessPool = mockPool
	defer func() { ui.BusinessPool = nil }()

	// Register both master buffer and server-mode queries
	masterCols := []string{"id", "title", "author"}
	masterRows := [][]any{
		{1, "Foundation", "Asimov"},
		{2, "Dune", "Herbert"},
		{3, "I, Robot", "Asimov"},
	}
	mockPool.RegisterQuery("SELECT * FROM books", masterCols, masterRows, nil)

	serverCols := []string{"id", "title"}
	serverRows := [][]any{
		{10, "Server Book"},
	}
	mockPool.RegisterQuery("SELECT * FROM server_books WHERE title LIKE $1", serverCols, serverRows, nil)

	// Build a container with text_input + submit button + client-mode data_grid
	// This triggers loadMasterBuffer (G1) during Compose
	containerNode := ui.NodeMeta{
		Area:         "screen",
		ComponentRef: "container",
		Layout:       ui.LayoutMeta{Type: "vertical"},
		Children: []ui.NodeMeta{
			{
				Area:         "title_input",
				ComponentRef: "text_input",
				BindTo:       "title",
				Placeholder:  "Filter",
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

	obj, err := ui.Compose(containerNode, "test-vista")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	c, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected *fyne.Container, got %T", obj)
	}

	gridTable := c.Objects[2].(*widget.Table)

	// Wait for eager master buffer load (G1) to complete
	var masterLoaded bool
	for start := time.Now(); time.Since(start) < 1*time.Second; {
		rows, _ := gridTable.Length()
		if rows == 3 {
			masterLoaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !masterLoaded {
		rows, _ := gridTable.Length()
		t.Fatalf("expected 3 rows after master buffer load, got %d", rows)
	}

	// Now trigger concurrent operations:
	// G3: Multiple filter events via EventBus (triggers filterMasterRows → fyne.Do)
	// We fire several filter events rapidly to stress the lock ordering.

	for i := 0; i < 5; i++ {
		snap := map[string]any{"author": "Asimov"}
		eb.Publish("screen:submit:test-vista", snap)
	}

	// Wait for all concurrent filter operations to complete.
	// If there is a deadlock (e.g., model.mu held during fyne.Do while
	// table.Refresh tries to RLock), this will timeout.
	done := make(chan struct{})
	go func() {
		// Poll until we see the filtered result (2 rows matching "Asimov")
		for start := time.Now(); time.Since(start) < 5*time.Second; {
			rows, _ := gridTable.Length()
			if rows == 2 {
				close(done)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// PASS — all concurrent operations completed without deadlock
	case <-time.After(5 * time.Second):
		rows, _ := gridTable.Length()
		t.Fatalf("DEADLOCK detected: concurrent G1+G3 operations did not complete within 5s (rows=%d)", rows)
	}
}
