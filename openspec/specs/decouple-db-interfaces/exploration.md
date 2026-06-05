## Exploration: decouple-db-interfaces

### Current State
`pkg/db/db.go` initializes two concrete connection pools `CorePool` and `BusinessPool` of type `*pgxpool.Pool` inside the `DB` struct:
```go
type DB struct {
	CorePool     *pgxpool.Pool
	BusinessPool *pgxpool.Pool
}
```
Currently, the codebase does not perform active queries on these pools. However, they are directly exposed to the application. This concrete dependency makes it impossible to run unit or integration tests without spinning up a live PostgreSQL instance, violating Capa 1 decoupling guidelines.

### Affected Areas
- `pkg/db/db.go` — Exposes concrete pool types. Needs to be updated to define and expose generic interfaces.
- `pkg/db/mock_db.go` — New file to be added to implement mock connection pools, query execution, rows scanning, and batch results for tests.
- `pkg/db/db_test.go` — Existing tests need to be verified to ensure they compile and run properly with the updated `DB` struct.
- `cmd/golemui/main.go` — Instantiates `db.DB` during client bootstrap. Must remain compatible without breaking.

### Approaches
1. **Broad DatabasePool Interface** — Define a single broad `DatabasePool` interface that abstracts all querying, execution, transaction, ping, and close methods of `pgxpool.Pool`.
   - Pros: Direct drop-in replacement; simplifies the `DB` struct definition since both pools share the exact same interface type.
   - Cons: Slightly violates the Interface Segregation Principle (ISP) if some consumers only need execution or querying capabilities.
   - Effort: Low

2. **Segregated DBQuerier and DatabasePool Interfaces** — Define `DBQuerier` strictly for query execution methods (`Query`, `QueryRow`, `Exec`, `SendBatch`). Define `DatabasePool` as an extending interface that embeds `DBQuerier` and adds connection/lifecycle controls (`Ping`, `Close`).
   - Pros: Adheres to the Interface Segregation Principle (ISP); callers that perform queries only require `DBQuerier`, simplifying mocking in business modules.
   - Cons: Adds a minor amount of boilerplate interface nesting.
   - Effort: Low

### Recommendation
We recommend **Approach 2 (Segregated Interfaces)**. It achieves the best clean-architecture separation. The `DB` struct fields `CorePool` and `BusinessPool` will expose the `DatabasePool` interface, ensuring that initialization (`Ping`, `Close`) is fully supported, while other consumers can accept a narrowed `DBQuerier` interface for maximum isolation.

We will also implement a thread-safe `MockDBPool` struct in a new file `pkg/db/mock_db.go`, alongside helper mock structs `MockRows` and `MockRow` to allow precise stubbing of database queries.

### Risks
- **pgx/v5 API Updates**: The interface signatures must match the `pgx/v5` types exactly (e.g., `pgx.Rows`, `pgx.Row`, `pgx.BatchResults`, and `pgconn.CommandTag`). Any future updates in the `pgx/v5` module might require updating the interface and mocks.
- **Nil checks/casting in caller code**: If any downstream code attempts to perform concrete type assertions back to `*pgxpool.Pool`, it will panic. Since no existing files do this, the risk is minimal.

### Ready for Proposal
Yes
