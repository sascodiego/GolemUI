# Spec: CLI Flags for GolemUI Bootstrap

See domain delta spec: [specs/client-bootstrap/spec.md](specs/client-bootstrap/spec.md)

## Summary

| Domain | Type | Requirements | Scenarios |
|--------|------|-------------|-----------|
| client-bootstrap | Delta (ADDED only) | 3 added, 0 modified, 0 removed | 7 |

## Coverage

- Happy paths: covered (custom config, override wins, defaults)
- Edge cases: covered (missing file, empty override chain)
- Error states: covered (file-not-found at custom path)
- Backward compat: covered (6 existing tests + REFACTOR phase)
