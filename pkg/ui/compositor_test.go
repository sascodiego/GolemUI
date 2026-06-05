package ui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/jackc/pgx/v5"
	"GolemUI/pkg/db"
	"GolemUI/pkg/eventbus"
	"GolemUI/pkg/ui"
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

	obj, err := ui.Compose(node)
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

	obj, err := ui.Compose(node)
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

	obj, err := ui.Compose(node)
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

	obj, err := ui.Compose(node)
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

	obj, err := ui.Compose(node)
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
	ui.BusinessPool = nil

	node := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		DataSource:   "SELECT 1",
	}

	// This should not crash / panic
	obj, err := ui.Compose(node)
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
	ctx  context.Context
	sql  string
	args []any
}

type trackingMockDBPool struct {
	*db.MockDBPool
	mu            sync.Mutex
	queriesCalled []queryCall
}

func (t *trackingMockDBPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.mu.Lock()
	t.queriesCalled = append(t.queriesCalled, queryCall{ctx: ctx, sql: sql, args: args})
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

	inputNode := ui.NodeMeta{
		Area:         "input_area",
		ComponentRef: "text_input",
		BindTo:       "filter_channel",
		Placeholder:  "Filter books",
	}

	gridNode := ui.NodeMeta{
		Area:         "grid_area",
		ComponentRef: "data_grid",
		BindTo:       "filter_channel",
		DataSource:   "SELECT * FROM books WHERE title LIKE $1",
	}

	inputObj, err := ui.Compose(inputNode)
	if err != nil {
		t.Fatalf("failed to compose text_input: %v", err)
	}
	entry, ok := inputObj.(*widget.Entry)
	if !ok {
		t.Fatalf("expected *widget.Entry, got %T", inputObj)
	}

	gridObj, err := ui.Compose(gridNode)
	if err != nil {
		t.Fatalf("failed to compose data_grid: %v", err)
	}
	table, ok := gridObj.(*widget.Table)
	if !ok {
		t.Fatalf("expected *widget.Table, got %T", gridObj)
	}
	if table == nil {
		t.Fatal("expected non-nil *widget.Table")
	}

	// Type into the entry. This should trigger publishers and subscriber queries
	test.Type(entry, "Book A")

	// Wait for query to execute with the typed text
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
		t.Fatal("expected query to be executed with parameter 'Book A'")
	}

	// Verify the context was NOT cancelled for the successful final call
	if lastCall.ctx.Err() != nil {
		t.Error("expected successful query context not to be cancelled, but it was")
	}

	// Now verify rapid typing cancels previous context
	trackingPool.mu.Lock()
	trackingPool.queriesCalled = nil
	trackingPool.mu.Unlock()

	// Type rapidly
	test.Type(entry, "B")
	test.Type(entry, "C")
	test.Type(entry, "D")

	// Wait a bit to ensure queries were registered/triggered
	time.Sleep(200 * time.Millisecond)

	trackingPool.mu.Lock()
	callsAfter := trackingPool.queriesCalled
	trackingPool.mu.Unlock()

	// We expect multiple queries, and at least the earlier ones should have cancelled contexts
	if len(callsAfter) < 2 {
		t.Fatalf("expected at least 2 queries triggered during rapid typing, got %d", len(callsAfter))
	}

	// Verify that early queries have their context cancelled
	var cancelledCount int
	for i := 0; i < len(callsAfter)-1; i++ {
		time.Sleep(10 * time.Millisecond)
		if callsAfter[i].ctx.Err() != nil {
			cancelledCount++
		}
	}

	if cancelledCount == 0 {
		t.Error("expected at least one early query to be cancelled, got 0")
	}
}


