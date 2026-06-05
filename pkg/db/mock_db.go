package db

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type QueryStub struct {
	Rows *MockRows
	Err  error
}

type ExecStub struct {
	Tag pgconn.CommandTag
	Err error
}

type MockDBPool struct {
	mu           sync.RWMutex
	queries      map[string]*QueryStub
	execs        map[string]*ExecStub
	batchResults []any
	pingErr      error
	closed       bool
}

func NewMockDBPool() *MockDBPool {
	return &MockDBPool{
		queries: make(map[string]*QueryStub),
		execs:   make(map[string]*ExecStub),
	}
}

func (m *MockDBPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return pgconn.CommandTag{}, fmt.Errorf("pool is closed")
	}
	stub, exists := m.execs[sql]
	if !exists {
		return pgconn.CommandTag{}, fmt.Errorf("no mock registered for exec: %s", sql)
	}
	return stub.Tag, stub.Err
}

func (m *MockDBPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, fmt.Errorf("pool is closed")
	}
	stub, exists := m.queries[sql]
	if !exists {
		return nil, fmt.Errorf("no mock registered for query: %s", sql)
	}
	if stub.Err != nil {
		return nil, stub.Err
	}
	if stub.Rows != nil {
		stub.Rows.mu.Lock()
		columns := stub.Rows.columns
		rows := stub.Rows.rows
		err := stub.Rows.err
		stub.Rows.mu.Unlock()

		return &MockRows{
			columns: columns,
			rows:    rows,
			err:     err,
		}, nil
	}
	return nil, nil
}

func (m *MockDBPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return &MockRow{err: fmt.Errorf("pool is closed")}
	}
	stub, exists := m.queries[sql]
	if !exists {
		return &MockRow{err: fmt.Errorf("no mock registered for query: %s", sql)}
	}
	if stub.Err != nil {
		return &MockRow{err: stub.Err}
	}
	if stub.Rows != nil {
		stub.Rows.mu.Lock()
		defer stub.Rows.mu.Unlock()
		if len(stub.Rows.rows) > 0 {
			return &MockRow{row: stub.Rows.rows[0]}
		}
	}
	return &MockRow{err: pgx.ErrNoRows}
}

func (m *MockDBPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &MockBatchResults{
		results: m.batchResults,
	}
}

func (m *MockDBPool) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pingErr
}

func (m *MockDBPool) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockDBPool) RegisterQuery(sql string, columns []string, data [][]any, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries[sql] = &QueryStub{
		Rows: &MockRows{
			columns: columns,
			rows:    data,
		},
		Err: err,
	}
}

func (m *MockDBPool) RegisterExec(sql string, tag pgconn.CommandTag, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execs[sql] = &ExecStub{
		Tag: tag,
		Err: err,
	}
}

func (m *MockDBPool) RegisterPingError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingErr = err
}

func (m *MockDBPool) RegisterBatchResults(results []any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchResults = results
}

type MockRows struct {
	mu      sync.Mutex
	columns []string
	rows    [][]any
	cursor  int
	err     error
	closed  bool
}

func (m *MockRows) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockRows) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func (m *MockRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (m *MockRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (m *MockRows) Next() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.err != nil {
		return false
	}
	if m.cursor < len(m.rows) {
		m.cursor++
		return true
	}
	m.closed = true
	return false
}

func (m *MockRows) Scan(dest ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("rows are closed")
	}
	if m.cursor == 0 || m.cursor > len(m.rows) {
		return fmt.Errorf("Scan called without calling Next first or after Next returned false")
	}
	row := m.rows[m.cursor-1]
	err := scanRow(row, dest...)
	if err != nil {
		m.closed = true
		m.err = err
	}
	return err
}

func (m *MockRows) Values() ([]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("rows are closed")
	}
	if m.cursor == 0 || m.cursor > len(m.rows) {
		return nil, fmt.Errorf("Values called without calling Next first")
	}
	return m.rows[m.cursor-1], nil
}

func (m *MockRows) RawValues() [][]byte {
	return nil
}

func (m *MockRows) Conn() *pgx.Conn {
	return nil
}

type MockRow struct {
	err error
	row []any
}

func (m *MockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(m.row) == 0 {
		return pgx.ErrNoRows
	}
	return scanRow(m.row, dest...)
}

type MockBatchResults struct {
	mu      sync.Mutex
	results []any
	cursor  int
}

func (m *MockBatchResults) Exec() (pgconn.CommandTag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cursor >= len(m.results) {
		return pgconn.CommandTag{}, fmt.Errorf("no more results in batch")
	}
	res := m.results[m.cursor]
	m.cursor++
	if err, ok := res.(error); ok {
		return pgconn.CommandTag{}, err
	}
	if stub, ok := res.(*ExecStub); ok {
		return stub.Tag, stub.Err
	}
	return pgconn.CommandTag{}, fmt.Errorf("unexpected batch result type for Exec")
}

func (m *MockBatchResults) Query() (pgx.Rows, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cursor >= len(m.results) {
		return nil, fmt.Errorf("no more results in batch")
	}
	res := m.results[m.cursor]
	m.cursor++
	if err, ok := res.(error); ok {
		return nil, err
	}
	if stub, ok := res.(*QueryStub); ok {
		if stub.Err != nil {
			return nil, stub.Err
		}
		if stub.Rows != nil {
			return &MockRows{
				columns: stub.Rows.columns,
				rows:    stub.Rows.rows,
			}, nil
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected batch result type for Query")
}

func (m *MockBatchResults) QueryRow() pgx.Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cursor >= len(m.results) {
		return &MockRow{err: fmt.Errorf("no more results in batch")}
	}
	res := m.results[m.cursor]
	m.cursor++
	if err, ok := res.(error); ok {
		return &MockRow{err: err}
	}
	if stub, ok := res.(*QueryStub); ok {
		if stub.Err != nil {
			return &MockRow{err: stub.Err}
		}
		if stub.Rows != nil && len(stub.Rows.rows) > 0 {
			return &MockRow{row: stub.Rows.rows[0]}
		}
		return &MockRow{err: pgx.ErrNoRows}
	}
	return &MockRow{err: fmt.Errorf("unexpected batch result type for QueryRow")}
}

func (m *MockBatchResults) Close() error {
	return nil
}

func scanRow(row []any, dest ...any) error {
	if len(dest) > len(row) {
		return fmt.Errorf("scan destination count (%d) exceeds row column count (%d)", len(dest), len(row))
	}
	for i, d := range dest {
		if d == nil {
			continue
		}
		val := row[i]
		if val == nil {
			continue
		}

		destVal := reflect.ValueOf(d)
		if destVal.Kind() != reflect.Pointer || destVal.IsNil() {
			return fmt.Errorf("scan destination %d is not a valid pointer", i)
		}

		elem := destVal.Elem()
		srcVal := reflect.ValueOf(val)

		if srcVal.Type().AssignableTo(elem.Type()) {
			elem.Set(srcVal)
		} else if srcVal.Type().ConvertibleTo(elem.Type()) {
			elem.Set(srcVal.Convert(elem.Type()))
		} else {
			return fmt.Errorf("cannot scan type %s into %s", srcVal.Type(), elem.Type())
		}
	}
	return nil
}
