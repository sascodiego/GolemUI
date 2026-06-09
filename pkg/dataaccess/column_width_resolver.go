package dataaccess

import (
	"context"
	"log"
	"sync"

	"GolemUI/pkg/db"
	"GolemUI/pkg/ui"
)

// Compile-time interface check.
var _ ui.ColumnWidthResolver = (*SQLColumnWidthResolver)(nil)

// SQLColumnWidthResolver reads column-width metadata from Layer 2/3
// using the core database pool.
type SQLColumnWidthResolver struct {
	pool  db.DatabasePool
	cache sync.Map // key: "origen|header" → value: string
}

// NewColumnWidthResolver creates a resolver backed by the core pool.
func NewColumnWidthResolver(pool db.DatabasePool) *SQLColumnWidthResolver {
	return &SQLColumnWidthResolver{pool: pool}
}

// cacheKey builds a composite key for the sync.Map cache.
func cacheKey(origen, header string) string {
	return origen + "|" + header
}

// Resolve returns the effective width string for a column.
func (r *SQLColumnWidthResolver) Resolve(origen, header string) string {
	key := cacheKey(origen, header)

	// Check cache first
	if cached, ok := r.cache.Load(key); ok {
		return cached.(string)
	}

	ctx := context.Background()
	result := ""

	// Step 1: Layer 3 — golemui.mapeo_interfaz.column_width
	if r.pool != nil {
		var cw string
		err := r.pool.QueryRow(ctx,
			"SELECT column_width FROM golemui.mapeo_interfaz WHERE origen_id = $1 AND columna_fisica = $2",
			origen, header,
		).Scan(&cw)
		if err == nil && cw != "" {
			result = cw
		}
	}

	// Step 2: Layer 2 — golemui.componentes.default_column_width
	if result == "" && r.pool != nil {
		var dcw string
		err := r.pool.QueryRow(ctx,
			"SELECT default_column_width FROM golemui.componentes WHERE id = 'data_grid'",
		).Scan(&dcw)
		if err == nil && dcw != "" {
			result = dcw
		}
		if err != nil {
			log.Printf("[ColumnWidthResolver] Layer 2 lookup error: %v", err)
		}
	}

	// Cache the result (including empty string)
	r.cache.Store(key, result)
	return result
}
