<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Event Resources — Implementation Plan

> **Depends on:** [design.md](./design.md), [001_application](../001_application/plan.md) (completed)  
> **Estimated steps:** 6  
> **Status:** In progress (implementation exists, needs fixes and tests)

---

## Current State

The event resource implementation **already exists** across all layers:
- Server handlers: `eventexposure_server.go`, `eventsubscription_server.go`, `eventtype_server.go`
- Controllers: `eventexposure.go`, `eventsubscription.go`, `eventtype.go`
- Mappers: `mapper/eventexposure/out.go`, `mapper/eventsubscription/out.go`, `mapper/eventtype/out.go`
- Stores: `EventExposureStore`, `EventSubscriptionStore`, `EventTypeStore` in `stores.go`
- Routes: All registered in `server.go`
- Deprecated endpoints: All registered in `deprecated.go`
- Wiring: All controllers wired in `cmd/main.go`
- Build: `go build ./...` passes

### Known Issues to Fix

1. **No unit tests exist** — zero `_test.go` files in the entire spy-server module
2. **EventType full table scan** — `Get` and `GetStatus` perform full scans for every single-resource lookup (design §8 Issue 2)
3. **Shared EventTrigger code duplication** — identical `mapEventTrigger` in both exposure and subscription mappers (design §8 Issue 5)

---

## Step 1: EventExposure Mapper Tests

**Goal:** Unit tests for the EventExposure CRD → API mapper.

**Tasks:**
1. Create `internal/mapper/eventexposure/out_test.go`:
   - Test `MapResponse` with a fully populated EventExposure CRD
   - Test `MapResponseWithResourceName` produces `<group>--<team>--<name>` format
   - Test visibility enum mapping (`world`→`World`, `zone`→`Zone`, `enterprise`→`Enterprise`, unknown→UPPER)
   - Test approval strategy mapping (`auto`→`Auto`, `simple`→`Simple`, `fourEyes`→`FourEyes`, unknown→UPPER)
   - Test scopes mapping with EventTrigger (SelectionFilter + ResponseFilter)
   - Test SelectionFilter.Expression JSON unmarshal (`*apiextensionsv1.JSON` → `map[string]interface{}`)
   - Test nil scopes handling (empty slice)
   - Test team/application mapping when Application store lookup succeeds
   - Test team/application fallback when Application store lookup fails (namespace fallback)
   - Test nil/empty status conditions

**Deliverables:**
- Complete EventExposure mapper test suite

### 🚧 Gate 1: Quality Check + Test Coverage
- [ ] All tests pass
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for eventexposure mapper)

---

## Step 2: EventSubscription Mapper Tests

**Goal:** Unit tests for the EventSubscription CRD → API mapper.

**Tasks:**
1. Create `internal/mapper/eventsubscription/out_test.go`:
   - Test `MapResponse` with a fully populated EventSubscription CRD
   - Test `MapResponseWithResourceName` produces `<group>--<team>--<name>` format
   - Test delivery mapping (all fields, including `RedeliveriesPerSecond` `*int` → `int`)
   - Test delivery with nil `RedeliveriesPerSecond` (defaults to 0)
   - Test trigger mapping when `Spec.Trigger` is nil (optional)
   - Test trigger mapping with full EventTrigger (SelectionFilter + ResponseFilter)
   - Test team/application mapping via Requestor store lookup
   - Test team/application fallback when store lookup fails
   - Test approval mapping when `Status.Approval` is nil
   - Test approval mapping when Approval exists (status, decider, comment from latest decision)
   - Test approval mapping when Approval exists but has no decisions

**Deliverables:**
- Complete EventSubscription mapper test suite

### 🚧 Gate 2: Quality Check + Test Coverage
- [ ] All tests pass
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for eventsubscription mapper)

---

## Step 3: EventType Mapper Tests

**Goal:** Unit tests for the EventType CRD → API mapper.

**Tasks:**
1. Create `internal/mapper/eventtype/out_test.go`:
   - Test `MapResponse` with a fully populated EventType CRD
   - Test all fields mapped correctly (name, type, version, description, specification, active)
   - Test status mapping with conditions
   - Test with nil/empty conditions

**Deliverables:**
- Complete EventType mapper test suite

### 🚧 Gate 3: Quality Check + Test Coverage
- [ ] All tests pass
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for eventtype mapper)

---

## Step 4: Controller Tests

**Goal:** Unit tests for all three event controllers with mock stores.

**Tasks:**
1. Create `internal/controller/eventexposure_test.go`:
   - Test `Get` — happy path (store returns exposure, label matches)
   - Test `Get` — application label mismatch (returns error)
   - Test `Get` — store error (exposure not found)
   - Test `Get` — invalid applicationId format
   - Test `GetAll` — returns multiple items, filtered by namespace + app label
   - Test `GetAll` — empty result set
   - Test `GetStatus` — happy path
   - Test `GetStatus` — label mismatch
   - Test `GetSubscriptions` — filters subscriptions by eventType
   - Test `GetSubscriptions` — no matching subscriptions (empty result)
   - Test `GetSubscriptions` — exposure not found
2. Create `internal/controller/eventsubscription_test.go`:
   - Test `Get` — happy path (constructs `<appName>--<name>` correctly)
   - Test `Get` — application label mismatch
   - Test `Get` — store error
   - Test `GetAll` — returns multiple items, filtered
   - Test `GetAll` — empty result set
   - Test `GetStatus` — happy path (constructs full name correctly)
3. Create `internal/controller/eventtype_test.go`:
   - Test `Get` — matches by CRD name directly
   - Test `Get` — matches by `MakeEventTypeName(spec.type)` (dot-to-hyphen)
   - Test `Get` — not found (returns 404)
   - Test `GetAll` — returns all event types
   - Test `GetAll` — empty store
   - Test `GetStatus` — matches by name
   - Test `GetStatus` — not found

**Deliverables:**
- Complete controller test suite for all three event controllers

### 🚧 Gate 4: Quality Check + Test Coverage
- [ ] All tests pass
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for controller package)

---

## Step 5: Server Handler Tests

**Goal:** Unit tests for the HTTP handler layer.

**Tasks:**
1. Create `internal/server/eventexposure_server_test.go`:
   - Test `GetAllEventExposures` — parses query params, calls controller, sets headers
   - Test `GetEventExposure` — parses path params, returns JSON
   - Test `GetEventExposureStatus` — returns status response
   - Test `GetEventExposureSubscriptions` — returns subscription list
   - Test error handling (controller returns error → Problem Details)
2. Create `internal/server/eventsubscription_server_test.go`:
   - Test `GetAllEventSubscriptions` — parses params, calls controller
   - Test `GetEventSubscription` — parses path params
   - Test `GetEventSubscriptionStatus` — returns status
   - Test error handling
3. Create `internal/server/eventtype_server_test.go`:
   - Test `GetAllEventTypes` — no applicationId param
   - Test `GetEventType` — parses eventTypeName
   - Test `GetEventTypeStatus` — returns status
   - Test error handling

**Deliverables:**
- Complete server handler test suite

### 🚧 Gate 5: Quality Check + Test Coverage
- [ ] All tests pass
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 70% for server package)

---

## Step 6: Documentation & Cleanup

**Goal:** Final documentation, license headers, and cleanup.

**Tasks:**
1. Add SPDX license headers to all new test files
2. Run `pre-commit run --all-files` and fix any issues
3. Run `golangci-lint run ./...` and fix any issues
4. Verify REUSE compliance
5. Document deferred items as follow-up TODOs:
   - [ ] EventType security scoping (if tenant isolation needed)
   - [ ] EventType Get optimization (secondary index instead of full scan)
   - [ ] Extract shared EventTrigger mapping to common package
   - [ ] Obfuscated access type support

**Deliverables:**
- Clean, documented, tested event implementation
- All license headers in place
- Deferred items documented

### 🚧 Gate 6: Final Review
- [ ] `go build ./...` passes
- [ ] All tests pass
- [ ] `pre-commit run --all-files` passes
- [ ] Run **quality-check** skill (final pass)
- [ ] Run **test-coverage** skill (final: 70% overall minimum)

---

## Summary

| Step | Description | Depends On | Gate |
|------|-------------|------------|------|
| 1 | EventExposure mapper tests | Existing code | Tests + 80% coverage |
| 2 | EventSubscription mapper tests | Existing code | Tests + 80% coverage |
| 3 | EventType mapper tests | Existing code | Tests + 80% coverage |
| 4 | Controller tests | Steps 1-3 | Tests + 80% coverage |
| 5 | Server handler tests | Step 4 | Tests + 70% coverage |
| 6 | Documentation + cleanup | Step 5 | Linting + REUSE + final coverage |

**Parallelism:** Steps 1, 2, and 3 are independent — mapper tests can be written in parallel. Step 4 depends on understanding the mappers. Steps 5-6 are sequential.

**Estimated effort:** Medium — the implementation already exists and builds. The work is primarily testing and documentation, with no code changes needed to the core implementation.
