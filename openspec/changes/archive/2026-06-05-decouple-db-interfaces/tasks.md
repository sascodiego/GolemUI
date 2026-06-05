# Task Breakdown: Decouple DB Interfaces (decouple-db-interfaces)

This artifact tracks the step-by-step implementation plan for decoupling the database backend from concrete connection pools.

## Review Workload Forecast
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: stacked-to-main
400-line budget risk: Low

---

## Task Breakdown

### Phase 1: Define Interfaces & Refactor DB Struct
- [x] Define `DBQuerier` and `DatabasePool` interfaces in [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go).
- [x] Update `DB` struct fields `CorePool` and `BusinessPool` to use the `DatabasePool` interface in [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go).
- [x] Refactor `InitDB` in [pkg/db/db.go](file:///src/GolemUI/pkg/db/db.go) to return the updated `DB` struct and verify connection health via the interface methods.

### Phase 2: Mock Database Infrastructure
- [x] Create [pkg/db/mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go).
- [x] Implement `MockDBPool` satisfying `DatabasePool` in [pkg/db/mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go).
- [x] Implement `MockRows` satisfying `pgx.Rows` and `MockRow` satisfying `pgx.Row` in [pkg/db/mock_db.go](file:///src/GolemUI/pkg/db/mock_db.go).
- [x] Add query/execution stubbing to `MockDBPool` for mock query registration.

### Phase 3: Testing & Verification
- [x] Create unit tests in [pkg/db/db_test.go](file:///src/GolemUI/pkg/db/db_test.go) verifying `MockDBPool` concurrent safety, stubbed responses, and error handling.
- [x] Run `go test ./...` to verify that `pkg/db/db_test.go` and `cmd/golemui/main_test.go` compile and pass correctly.
