# Design: Decouple DB Interfaces (decouple-db-interfaces)

This document defines the interface layer to decouple GolemUI from concrete pgx connection pools, introducing generic interfaces and a mock connection pool.

---

## 1. Architectural Decisions

| Decision | Approach | Rationale |
| :--- | :--- | :--- |
| **Interface Segregation** | Define `DBQuerier` and `DatabasePool` in `pkg/db/db.go`. | Separates query capability from pool lifecycle management. Allows consumers to accept a narrowed `DBQuerier` for operations. |
| **Implicit Interface Go Bindings** | Leverage Go's implicit interfaces to keep pgxpool unmodified. | Concrete `*pgxpool.Pool` connections satisfy `DatabasePool` out of the box without wrapper structs. |
| **Mock Database Pool** | Introduce thread-safe `MockDBPool`, `MockRows`, and `MockRow` in `pkg/db/mock_db.go`. | Enables isolated unit and integration testing without a running Postgres daemon. |

---

## 2. Segregated Go Interfaces

### `pkg/db/db.go` Interface Definition

```go
package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBQuerier abstracts standard SQL query execution methods from pgx.
type DBQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// DatabasePool extends DBQuerier with pool lifecycle and health-check methods.
type DatabasePool interface {
	DBQuerier
	Ping(ctx context.Context) error
	Close()
}
```

---

## 3. Structure of `pkg/db/db.go`

The `DB` struct fields are changed from concrete pools to the `DatabasePool` interface.

```go
type DB struct {
	CorePool     DatabasePool
	BusinessPool DatabasePool
}
```

### Decoupled Initialization (`InitDB`)

```go
func InitDB(ctx context.Context, coreCfg Config, bizCfg Config) (*DB, error) {
	// 1. Establish concrete connections
	corePool, err := pgxpool.NewWithConfig(ctx, coreConfig)
	// 2. Establish biz connections
	bizPool, err := pgxpool.NewWithConfig(ctx, bizConfig)
	
	// 3. Ping and verify via the DatabasePool interface
	if err := corePool.Ping(ctx); err != nil { ... }
	if err := bizPool.Ping(ctx); err != nil { ... }

	return &DB{
		CorePool:     corePool, // Satisfies DatabasePool implicitly
		BusinessPool: bizPool, // Satisfies DatabasePool implicitly
	}, nil
}
```

---

## 4. Mock Database Infrastructure (`pkg/db/mock_db.go`)

### Mock Pool Structure

```go
package db

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type MockDBPool struct {
	mu        sync.RWMutex
	queries   map[string]*QueryStub
	execs     map[string]*ExecStub
	pingErr   error
	closed    bool
}

type QueryStub struct {
	Rows *MockRows
	Err  error
}

type ExecStub struct {
	Tag pgconn.CommandTag
	Err error
}
```

### Mock Rows Structure

```go
type MockRows struct {
	mu      sync.Mutex
	columns []string
	rows    [][]any
	cursor  int
	err     error
	closed  bool
}

type MockRow struct {
	err error
	row []any
}
```

---

## 5. Testing Strategy

1. **Unit Tests (`pkg/db/db_test.go`)**:
   - Verify `InitDB` failure modes.
   - Verify `MockDBPool` behaves correctly under concurrent access (using race detector).
2. **Mock Scenarios**:
   - Register a mock query returning multiple `MockRows`. Iterate and check standard scan operations.
   - Register query returning an error and assert `Query` propagation.
   - Register an exec operation returning specific `pgconn.CommandTag` and assert matching output.
