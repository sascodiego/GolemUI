package ui

import "context"

// DataSet is the clean data contract returned by DataSource.
// Layer 4 receives headers + string rows + optional column-width hints.
// No database driver types leak through.
type DataSet struct {
	// Headers contains the column names from the query result.
	// Length determines the number of columns.
	Headers []string

	// Rows contains the cell values preserving native Go types.
	// Each inner slice has the same length as Headers.
	// Values may be int64, float64, bool, string, or nil.
	Rows [][]any

	// ColumnWidths contains per-column width hints from metadata.
	// Each entry corresponds to Headers[i] by index.
	// Values follow the metric convention: "150px", "1fr", "auto", or "".
	// Nil or empty string means "use fallback resolution".
	ColumnWidths []string
}

// DataSource replaces direct BusinessPool access in the compositor.
// Implementations own the pool, SQL execution, driver-value normalization
// (FormatValue), and argument extraction (ExtractOrderedArgs).
// The renderer never sees database driver types or SQL strings.
type DataSource interface {
	// Fetch executes a data-source query with positional arguments.
	Fetch(ctx context.Context, source string, args ...any) (DataSet, error)

	// FetchAll loads all data without filter arguments (client-mode master buffer).
	// Equivalent to Fetch(ctx, source) with no args.
	FetchAll(ctx context.Context, source string) (DataSet, error)
}

// ColumnWidthResolver reads column-width metadata from Layer 2/3.
// origen matches vistas_consulta.origen_datos; header matches the
// column name returned in DataSet.Headers.
type ColumnWidthResolver interface {
	// Resolve returns the effective width string for a column.
	Resolve(origen string, header string) string
}

// MockDataSource is a test double for compositor tests.
// It records calls and returns canned DataSet values.
type MockDataSource struct {
	FetchCalled bool
	FetchSource string
	FetchArgs   []any
	FetchResult DataSet
	FetchError  error

	FetchAllCalled bool
	FetchAllSource string
	FetchAllResult DataSet
	FetchAllError  error
}

func (m *MockDataSource) Fetch(_ context.Context, source string, args ...any) (DataSet, error) {
	m.FetchCalled = true
	m.FetchSource = source
	m.FetchArgs = args
	return m.FetchResult, m.FetchError
}

func (m *MockDataSource) FetchAll(_ context.Context, source string) (DataSet, error) {
	m.FetchAllCalled = true
	m.FetchAllSource = source
	return m.FetchAllResult, m.FetchAllError
}

// MockCWR is a test double for ColumnWidthResolver in compositor tests.
type MockCWR struct {
	ResolveFunc func(origen, header string) string
}

func (m *MockCWR) Resolve(origen, header string) string {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(origen, header)
	}
	return ""
}
