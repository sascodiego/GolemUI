package dataaccess

import (
	"context"
	"fmt"
	"log"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
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

	var dataRows [][]string
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
		stringRow := make([]string, len(vals))
		for i, val := range vals {
			stringRow[i] = FormatValue(val)
		}
		dataRows = append(dataRows, stringRow)
	}

	log.Printf("[DataAccess] Query successful. Loaded %d columns, %d rows.", len(headers), len(dataRows))
	return ui.DataSet{Headers: headers, Rows: dataRows}, nil
}

// FetchAll loads all data without filter arguments.
func (s *SQLDataSource) FetchAll(ctx context.Context, source string) (ui.DataSet, error) {
	return s.Fetch(ctx, source)
}
