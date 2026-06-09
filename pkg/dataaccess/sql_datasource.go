package dataaccess

import (
	"context"
	"fmt"
	"log"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"

	"github.com/jackc/pgx/v5/pgtype"
)

// Compile-time interface check.
var _ ui.DataSource = (*SQLDataSource)(nil)

// SQLDataSource implements ui.DataSource using a single db.DatabasePool.
type SQLDataSource struct {
	pool db.DatabasePool
}

// NewSQLDataSource creates a DataSource backed by the given pool.
func NewSQLDataSource(pool db.DatabasePool) *SQLDataSource {
	return &SQLDataSource{pool: pool}
}

// unwrapNumeric converts pgtype.Numeric to float64 at the fetch boundary.
// pgx returns pgtype.Numeric for NUMERIC/DECIMAL columns; unwrapping
// ensures a plain Go type enters the transport pipeline.
// All other types pass through unchanged.
func unwrapNumeric(val any) any {
	if pn, ok := val.(*pgtype.Numeric); ok {
		f64, err := pn.Float64Value()
		if err == nil {
			return f64.Float64
		}
	}
	if pn, ok := val.(pgtype.Numeric); ok {
		f64, err := pn.Float64Value()
		if err == nil {
			return f64.Float64
		}
	}
	return val
}

// UnwrapNumeric exposes unwrapNumeric for testing.
func UnwrapNumeric(val any) any {
	return unwrapNumeric(val)
}

// Fetch executes a data-source query with positional arguments.
func (s *SQLDataSource) Fetch(ctx context.Context, source string, args ...any) (ui.DataSet, error) {
	if source == "" {
		return ui.DataSet{}, fmt.Errorf("dataaccess: empty source")
	}
	if s.pool == nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: pool is nil")
	}
	if err := ctx.Err(); err != nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: context cancelled before query: %w", err)
	}

	rows, err := s.pool.Query(ctx, source, args...)
	if err != nil {
		return ui.DataSet{}, fmt.Errorf("dataaccess: query failed: %w", err)
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	headers := make([]string, len(fds))
	for i, fd := range fds {
		headers[i] = fd.Name
	}

	var dataRows [][]any
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			log.Printf("[DataAccess] Context cancelled during row scan: %v", err)
			return ui.DataSet{}, fmt.Errorf("dataaccess: context cancelled during scan: %w", err)
		}
		vals, err := rows.Values()
		if err != nil {
			log.Printf("[DataAccess] Error scanning row values: %v", err)
			break
		}
		anyRow := make([]any, len(vals))
		for i, val := range vals {
			anyRow[i] = unwrapNumeric(val)
		}
		dataRows = append(dataRows, anyRow)
	}

	log.Printf("[DataAccess] Query successful. Loaded %d columns, %d rows.", len(headers), len(dataRows))
	return ui.DataSet{Headers: headers, Rows: dataRows}, nil
}

// FetchAll loads all data without filter arguments.
func (s *SQLDataSource) FetchAll(ctx context.Context, source string) (ui.DataSet, error) {
	return s.Fetch(ctx, source)
}
