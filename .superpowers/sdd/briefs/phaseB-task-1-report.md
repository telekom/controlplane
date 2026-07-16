## Phase B Task 1 Report: Zone selection + naming + EventConfig utilities

### Status: DONE

### Files Created
- `spectre/internal/handler/util/zone.go` - `GetListeningZone` implementing legacy preference logic
- `spectre/internal/handler/util/eventconfig.go` - `GetEventConfig` resolving EventConfig for a zone
- `spectre/internal/handler/util/naming.go` - Deterministic CR naming helpers + constants
- `spectre/internal/handler/util/suite_test.go` - Ginkgo test suite bootstrap
- `spectre/internal/handler/util/zone_test.go` - Zone selection + EventConfig tests (8 specs)
- `spectre/internal/handler/util/naming_test.go` - Naming helper tests (19 specs)

### Implementation Notes

**GetListeningZone** ports the legacy `ListenerUtil.getListeningZone` preference:
1. Listener zone with EventConfig that supports provider → use listener zone
2. Listener zone with EventConfig that supports consumer → use listener zone  
3. Provider zone has EventConfig → use provider zone
4. Consumer zone has EventConfig → use consumer zone
5. None → `BlockedErrorf`

Zone support check uses `EventConfig.SupportsZone(zoneName)` which checks `Spec.Zone.Name` match, then `Mesh.FullMesh`, then `Mesh.ZoneNames` containment.

**GetEventConfig** uses `client.MatchingFields{EventConfigZoneIndex: zone.Name}` for efficient lookup (same field index as event domain). Returns `BlockedError` if not found or not ready.

**Naming helpers** use `labelutil.NormalizeNameValue` (preserves dots, replaces `/`/`_`/`\`/spaces with `-`, lowercases, truncates at 253 chars with hash if needed).

**Note on NormalizeNameValue behavior**: Dots are NOT replaced (only `/`, `_`, `\`, and spaces become `-`). This means `MakePublisherName("de.telekom.ei.listener.app")` produces `"de.telekom.ei.listener.app"` with dots preserved. This is fine for K8s resource names (dots are allowed in DNS subdomain names up to 253 chars).

### Test Coverage
- 27 specs total, all passing
- 96.8% statement coverage for `spectre/internal/handler/util`
- `make build test` passes cleanly

### Dependencies Added
- `github.com/stretchr/testify` (already indirect via mockery-generated fakes in common)
- `github.com/pkg/errors` (already in go.mod)
- No new external dependencies; only uses existing module imports
