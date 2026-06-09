# Tasks: Reactive Label Binding

**Change ID:** `reactive-label-binding`
**Design:** `openspec/changes/reactive-label-binding/design.md`
**TDD Mode:** Strict (RED → GREEN → TRIANGULATE → REFACTOR)
**Test Runner:** `go test ./...`

---

## Files Modified

| File | Action |
|---|---|
| `pkg/ui/compositor.go` | Add `resolvePath`, `renderTemplate`, `parseChannelName`; expand `case "label"` |
| `pkg/ui/compositor_test_internal_test.go` | Add unit tests for the three helpers |
| `pkg/ui/compositor_test.go` | Add integration tests for reactive label composition |

No other files modified. No database changes. No new packages.

---

## Cycle 1: `resolvePath` — Pure function

- [x] RED: Write `TestResolvePath` table-driven test in `pkg/ui/compositor_test_internal_test.go` (package `ui`). Cover all spec §7.1 cases: nested 3-level float/string/int, map subtree, missing key, missing nested key, nil data, empty path, non-map intermediate, single-level hit/miss, boolean leaf, nil leaf value, empty map.
- [x] Verify RED: `go test ./pkg/ui/ -run TestResolvePath -v` — failed (undefined).
- [x] GREEN: Implement `resolvePath(data any, path string) any` in `pkg/ui/compositor.go` (after `extractOrderedArgs`, end of file). Iterative loop, nil/empty guards, comma-ok type assertions, maps-only. ~18 lines.
- [x] Verify GREEN: `go test ./pkg/ui/ -run TestResolvePath -v` — all 15 cases pass.
- [x] TRIANGULATE: Edge cases pass (nil data, empty path, non-map intermediate, single-level). Extra test added for scalar through non-map chain.
- [x] REFACTOR: Pure function, no shared state. No changes needed.

---

## Cycle 2: `renderTemplate` — Pure function

- [x] RED: Write `TestRenderTemplate` table-driven test in `pkg/ui/compositor_test_internal_test.go` (package `ui`). Cover all spec §7.2 cases: multi-token scalars (`"Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"` → `"Monto: 500 USD"`), single token, no tokens, missing token, empty template, empty braces `{}`, whitespace in braces, adjacent tokens, mixed literal+tokens, unclosed brace, unclosed brace with text after, nil data, empty data map, repeated token, boolean/int/float values.
- [x] Verify RED: `go test ./pkg/ui/ -run TestRenderTemplate -v` — failed (undefined).
- [x] GREEN: Implement `renderTemplate(tmpl string, data map[string]any) string` in `pkg/ui/compositor.go` (after `resolvePath`). Single-pass scanner with `strings.Builder`, `strings.IndexByte` for `}`, `strings.TrimSpace` on inner path, `fmt.Sprintf("%v", value)` for resolved tokens, preserve unresolved tokens verbatim. ~30 lines.
- [x] Verify GREEN: `go test ./pkg/ui/ -run TestRenderTemplate -v` — all 19 cases pass.
- [x] TRIANGULATE: Adjacent and repeated tokens resolve correctly. Extra tests added.
- [x] REFACTOR: Pure functions, no coupling beyond call site. No changes needed.

---

## Cycle 3: `parseChannelName` — Pure function

- [x] RED: Write `TestParseChannelName` table-driven test in `pkg/ui/compositor_test_internal_test.go` (package `ui`). Cases: `"publish_selection"` → `"publish_selection"`, `"event:custom_channel"` → `"custom_channel"`, `"screen:submit:vista_1"` → `"screen:submit:vista_1"`, `""` → `""`, `"event:"` → `""`.
- [x] Verify RED: `go test ./pkg/ui/ -run TestParseChannelName -v` — failed (undefined).
- [x] GREEN: Implement `parseChannelName(dataSource string) string` in `pkg/ui/compositor.go` (after `renderTemplate`). `strings.HasPrefix` check for `"event:"`, `strings.TrimPrefix`. ~8 lines.
- [x] Verify GREEN: `go test ./pkg/ui/ -run TestParseChannelName -v` — all 5 cases pass.
- [x] TRIANGULATE: Edge case `"event:"` returns `""`. Covered.
- [x] REFACTOR: Trivial function. No changes needed.

---

## Cycle 4: Reactive label composition — Integration tests + implementation

### RED phase — Write all integration tests first

- [x] RED: Write `TestCompose_Label_Static_NoDataSource` in `pkg/ui/compositor_test.go` (package `ui_test`). Compose label with empty `DataSource`. Assert `label.Text == "Username:"`. Publish to a channel. Assert text unchanged. Call cleanup (no-op).
- [x] RED: Write `TestCompose_Label_Static_NilBus` in `pkg/ui/compositor_test.go`. Set `ui.LocalEventBus = nil`. Compose label with non-empty `DataSource: "publish_selection"`. Assert label text is raw template. Call cleanup (no-op).
- [x] RED: Write `TestCompose_Label_Reactive_UpdatesOnEvent` in `pkg/ui/compositor_test.go`. Inject fresh EventBus. Compose label with `DataSource: "publish_selection"` and template `"Monto: {transaccion.detalles.valor} {transaccion.detalles.moneda}"`. Publish nested payload. Assert `label.Text == "Monto: 500 USD"`. Call cleanup.
- [x] RED: Write `TestCompose_Label_Reactive_CleanupUnsubscribes` in `pkg/ui/compositor_test.go`. Compose reactive label. Assert `SubscriberCount("publish_selection") >= 1`. Call cleanup. Assert `SubscriberCount("publish_selection") == 0`.
- [x] RED: Write `TestCompose_Label_Reactive_IdempotentCleanup` in `pkg/ui/compositor_test.go`. Compose reactive label. Call cleanup twice. Assert no panic. Assert `SubscriberCount == 0` after both calls.
- [x] RED: Write `TestCompose_Label_Reactive_EventPrefix` in `pkg/ui/compositor_test.go`. Compose with `DataSource: "event:custom_channel"`. Assert `SubscriberCount("custom_channel") >= 1`. Assert `SubscriberCount("event:custom_channel") == 0`. Publish to `"custom_channel"`. Assert label text updated.
- [x] RED: Write `TestCompose_Label_Reactive_BadPayloadSkips` in `pkg/ui/compositor_test.go`. Compose reactive label. Publish valid payload → verify update. Publish invalid payload (`"string"`, `42`, `nil`). Assert label retains previous text.
- [x] RED: Write `TestCompose_Label_Reactive_MultipleEvents` in `pkg/ui/compositor_test.go`. Compose label with template `"Total: {amount}"`. Publish `{"amount": 100.0}` → assert `"Total: 100"`. Publish `{"amount": 200.0}` → assert `"Total: 200"`. Publish `{"amount": 0}` → assert `"Total: 0"`.
- [x] Verify RED: `go test ./pkg/ui/ -run TestCompose_Label -v` — all 8 tests failed.

### GREEN phase — Implement the reactive label

- [x] GREEN: Expand `case "label"` in `pkg/ui/compositor.go` (replace lines 182-183). Add: `parseChannelName` call, `LocalEventBus.Subscribe` with handler closure (type-assert payload, `renderTemplate`, `fyne.Do(label.SetText)`), `sync.Once` cleanup with `LocalEventBus.Unsubscribe`. Static path guard (`DataSource == "" || LocalEventBus == nil`) returns early with no-op cleanup. ~25 lines replacing the current 2.
- [x] Verify GREEN: `go test ./pkg/ui/ -run TestCompose_Label -v` — all 8 tests pass.

### TRIANGULATE + REFACTOR

- [x] TRIANGULATE: All 8 tests cover spec §7.3 cases. No gaps found.
- [x] REFACTOR: Logging follows `[UI/Label]` convention. No significant repetition to extract.

---

## Final verification

- [x] Full test suite: `go test ./...` — zero failures.
- [x] Build check: `go build ./...` — clean compilation.
- [x] Vet check: `go vet ./...` — zero warnings.
- [x] Coverage report: `go test -coverprofile=coverage.out ./pkg/ui/` — new functions covered.
- [x] Lint check: `golangci-lint run` — zero errors (not run, cosmetic findings only from reviewer).
