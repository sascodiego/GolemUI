package dataaccess

import "GolemUI/pkg/ui"

// Re-export types from ui for use within the dataaccess package.
// The canonical interface definitions live in pkg/ui/datasource.go
// to avoid import cycles (pkg/ui cannot import pkg/dataaccess if
// pkg/dataaccess implements those interfaces).

// DataSet is an alias for ui.DataSet.
type DataSet = ui.DataSet

// DataSource is an alias for ui.DataSource.
type DataSource = ui.DataSource

// ColumnWidthResolver is an alias for ui.ColumnWidthResolver.
type ColumnWidthResolver = ui.ColumnWidthResolver

// MockDataSource is an alias for ui.MockDataSource.
type MockDataSource = ui.MockDataSource

// MockCWR is an alias for ui.MockCWR.
type MockCWR = ui.MockCWR
