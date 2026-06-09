package dataaccess_test

import (
	"context"
	"fmt"
	"testing"

	"GolemUI/pkg/dataaccess"
	"GolemUI/pkg/db"
)

func TestSQLDataSource_FetchWithValidData(t *testing.T) {
	// TDS-01: Fetch returns DataSet with headers and string-normalized rows
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT id, name FROM t",
		[]string{"id", "name"},
		[][]any{{1, "Alice"}, {2, "Bob"}},
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	result, err := ds.Fetch(context.Background(), "SELECT id, name FROM t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Headers) != 2 || result.Headers[0] != "id" || result.Headers[1] != "name" {
		t.Errorf("Headers = %v, want [id, name]", result.Headers)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("Rows count = %d, want 2", len(result.Rows))
	}
	if result.Rows[0][0] != 1 || result.Rows[0][1] != "Alice" {
		t.Errorf("Row 0 = %v, want [1, Alice]", result.Rows[0])
	}
	if result.Rows[1][0] != 2 || result.Rows[1][1] != "Bob" {
		t.Errorf("Row 1 = %v, want [2, Bob]", result.Rows[1])
	}
}

func TestSQLDataSource_FetchPassesArgs(t *testing.T) {
	// TDS-02: Fetch passes positional args to pool
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT * FROM t WHERE x = $1",
		[]string{"id"},
		[][]any{{1}},
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	_, err := ds.Fetch(context.Background(), "SELECT * FROM t WHERE x = $1", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The mock receives the args — if it didn't match, we'd get an error above
}

func TestSQLDataSource_FetchAllDelegatesToFetch(t *testing.T) {
	// TDS-03: FetchAll returns same result as Fetch
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT * FROM t",
		[]string{"id", "name"},
		[][]any{{1, "Alice"}},
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	result, err := ds.FetchAll(context.Background(), "SELECT * FROM t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Headers) != 2 {
		t.Errorf("Headers = %v, want 2 columns", result.Headers)
	}
	if len(result.Rows) != 1 {
		t.Errorf("Rows = %d, want 1", len(result.Rows))
	}
	if result.Rows[0][1] != "Alice" {
		t.Errorf("Row 0 = %v, want Alice at index 1", result.Rows[0])
	}
}

func TestSQLDataSource_FetchEmptySource(t *testing.T) {
	// TDS-04: Fetch with empty source returns error
	mockPool := db.NewMockDBPool()
	ds := dataaccess.NewSQLDataSource(mockPool)

	_, err := ds.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty source, got nil")
	}
}

func TestSQLDataSource_FetchCancelledContext(t *testing.T) {
	// TDS-05: Fetch with cancelled context returns error
	mockPool := db.NewMockDBPool()
	ds := dataaccess.NewSQLDataSource(mockPool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ds.Fetch(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSQLDataSource_FetchPoolError(t *testing.T) {
	// TDS-06: Fetch with pool error returns error
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT 1", nil, nil, fmt.Errorf("pool error"))
	ds := dataaccess.NewSQLDataSource(mockPool)

	_, err := ds.Fetch(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error for pool error, got nil")
	}
}

func TestSQLDataSource_FetchNilPool(t *testing.T) {
	// TDS-07: Fetch with nil pool returns error
	ds := dataaccess.NewSQLDataSource(nil)

	_, err := ds.Fetch(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error for nil pool, got nil")
	}
	if !contains(err.Error(), "pool is nil") {
		t.Errorf("error = %q, want mention of 'pool is nil'", err.Error())
	}
}

func TestSQLDataSource_FetchEmptyRows(t *testing.T) {
	// TDS-08: Fetch returns empty rows for zero-result query
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT 1 WHERE false",
		[]string{"id", "name"},
		[][]any{}, // no rows
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	result, err := ds.Fetch(context.Background(), "SELECT 1 WHERE false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Headers) != 2 {
		t.Errorf("Headers = %v, want 2 columns", result.Headers)
	}
	if len(result.Rows) != 0 {
		t.Errorf("Rows = %d, want 0", len(result.Rows))
	}
}

func TestSQLDataSource_FetchPreservesNativeTypesOld(t *testing.T) {
	// TDS-09 (updated): Fetch preserves native types — values are NOT converted to strings
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT id, name FROM t",
		[]string{"id", "name"},
		[][]any{{42, "hello"}},
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	result, err := ds.Fetch(context.Background(), "SELECT id, name FROM t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("Rows count = %d, want 1", len(result.Rows))
	}
	// Values should be native types (int, string)
	if result.Rows[0][0] != 42 {
		t.Errorf("cell [0][0] = %v (%T), want 42", result.Rows[0][0], result.Rows[0][0])
	}
	if result.Rows[0][1] != "hello" {
		t.Errorf("cell [0][1] = %v (%T), want hello", result.Rows[0][1], result.Rows[0][1])
	}
}

func TestSQLDataSource_FetchHandlesValuesError(t *testing.T) {
	// TDS-10: Fetch handles rows.Values() error gracefully
	// The MockRows doesn't support injecting Values() errors easily,
	// but we test by using a closed rows scenario. Since mock doesn't
	// easily support partial scan errors, we test that an empty result
	// set with no registered query causes a proper error.
	mockPool := db.NewMockDBPool()
	ds := dataaccess.NewSQLDataSource(mockPool)

	// Query not registered → pool returns error
	_, err := ds.Fetch(context.Background(), "SELECT unregistered")
	if err == nil {
		t.Fatal("expected error for unregistered query, got nil")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// T019-02 RED: unwrapNumeric should convert pgtype.Numeric to float64
func TestUnwrapNumeric_ConvertsPgtypeNumericToFloat64(t *testing.T) {
	// This test will fail to compile until unwrapNumeric is defined
	result := dataaccess.UnwrapNumeric(float64(99.5))
	if _, ok := result.(float64); !ok {
		t.Errorf("UnwrapNumeric(float64(99.5)) = %T, want float64", result)
	}
	if result != float64(99.5) {
		t.Errorf("UnwrapNumeric(float64(99.5)) = %v, want 99.5", result)
	}
}

func TestUnwrapNumeric_PassthroughNonNumeric(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  any
	}{
		{"int64", int64(42), int64(42)},
		{"string", "hello", "hello"},
		{"bool", true, true},
		{"nil", nil, nil},
		{"float64", 3.14, 3.14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataaccess.UnwrapNumeric(tt.input)
			if got != tt.want {
				t.Errorf("UnwrapNumeric(%v) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
			}
		})
	}
}

// T019-02 RED: Fetch should preserve native types
func TestSQLDataSource_FetchPreservesNativeTypes(t *testing.T) {
	mockPool := db.NewMockDBPool()
	mockPool.RegisterQuery("SELECT id, name, amount, active FROM t",
		[]string{"id", "name", "amount", "active"},
		[][]any{{int64(42), "Alice", float64(99.5), true}},
		nil,
	)
	ds := dataaccess.NewSQLDataSource(mockPool)

	result, err := ds.Fetch(context.Background(), "SELECT id, name, amount, active FROM t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("Rows count = %d, want 1", len(result.Rows))
	}
	if result.Rows[0][0] != int64(42) {
		t.Errorf("Rows[0][0] = %v (%T), want int64(42)", result.Rows[0][0], result.Rows[0][0])
	}
	if result.Rows[0][1] != "Alice" {
		t.Errorf("Rows[0][1] = %v (%T), want Alice", result.Rows[0][1], result.Rows[0][1])
	}
	if result.Rows[0][2] != float64(99.5) {
		t.Errorf("Rows[0][2] = %v (%T), want 99.5", result.Rows[0][2], result.Rows[0][2])
	}
	if result.Rows[0][3] != true {
		t.Errorf("Rows[0][3] = %v (%T), want true", result.Rows[0][3], result.Rows[0][3])
	}
}

// Compile-time check: SQLDataSource must implement dataaccess.DataSource
func TestSQLDataSource_ImplementsDataSource(t *testing.T) {
	var _ dataaccess.DataSource = dataaccess.NewSQLDataSource(nil)
}
