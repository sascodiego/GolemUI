# Proposal: Decouple DB Interfaces

## Intent
Decouple `pkg/db/db.go` from concrete `*pgxpool.Pool` connections by introducing segregated interfaces (`DBQuerier` and `DatabasePool`). This enables robust mocking/stubbing without requiring a real PostgreSQL instance during unit tests.

## Capabilities

| Capability Type | Capability ID | Description |
| :--- | :--- | :--- |
| **New** | `decoupled-database-abstraction` | Segregated interface layer representing connection pools and queriers to remove direct vendor-pool dependency. |
| **New** | `database-mocking-infrastructure` | Mocking structure implementing `DBQuerier` and `DatabasePool` (with `MockRows`, `MockRow`, and `MockDBPool`) for isolated testing. |
| **Modified** | `database-connection-management` | Core client updates to handle abstraction interfaces instead of raw connection pools. |

## Affected Areas

- [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go) - Define `DBQuerier`, `DatabasePool`, and adapt the configuration logic.
- [pkg/db/mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go) - New file containing mock implementations.
- [cmd/golemui/main.go](file:///src/GolemUI/cmd/golemui/main.go) - Adjust the `App` struct to reference `DatabasePool` or interface types.
- [pkg/db/db_test.go](file:///src/GolemUI/pkg/db/db_test.go) - Adjust test suites to verify new interface behavior.

## Approach
1. **Define Interfaces**:
   - `DBQuerier`: Wraps common query methods (`Query`, `QueryRow`, `Exec`, `Ping`).
   - `DatabasePool`: Extends `DBQuerier` with lifecycle operations (`Close`).
2. **Implement Mocks**:
   - Implement `MockDBPool`, `MockRows`, and `MockRow` to allow unit testing of components dependent on the database.
3. **Refactor Client**:
   - Modify structural definitions inside `pkg/db/db.go` to use these interfaces.
   - Update references in `cmd/golemui/main.go`.

## Success Criteria
- [ ] No compilation errors across all modules.
- [ ] Unit tests in `pkg/db/db_test.go` and `cmd/golemui/main_test.go` pass.
- [ ] Interface mocks are successfully stubbed in new or existing unit tests.

## Rollback Plan
- Run `git checkout -- pkg/db/db.go cmd/golemui/main.go pkg/db/db_test.go`
- Delete `pkg/db/mock_db.go`.
- Remove `openspec/changes/decouple-db-interfaces/proposal.md`.
