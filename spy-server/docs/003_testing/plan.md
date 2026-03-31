<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Testing Concept — Implementation Plan

> **Depends on:** [design.md](./design.md), [000_initial](../000_initial/plan.md) (completed), [001_application](../001_application/plan.md) (completed)  
> **Estimated steps:** 8  
> **Status:** Not started

---

## Step 1: Add Test Dependencies to go.mod

**Goal:** Add Ginkgo v2, Gomega, and go-snaps as test dependencies.

**Tasks:**
1. Add test framework dependencies:
   ```bash
   go get github.com/onsi/ginkgo/v2@v2.28.1
   go get github.com/onsi/gomega@v1.39.1
   go get github.com/gkampitakis/go-snaps@v0.5.21
   ```
2. Verify `go mod tidy` succeeds
3. Verify `go build ./...` still passes

**Deliverables:**
- Updated `go.mod` and `go.sum` with test dependencies
- Build still passes

### 🚧 Gate 1: Build Verification
- [ ] `go build ./...` passes
- [ ] `go mod tidy` produces no changes
- [ ] `ginkgo version` resolves correctly

---

## Step 2: Test Data Fixtures

**Goal:** Create JSON fixture files containing valid CRD objects for mock stores.

**Tasks:**
1. Create directory `test/mocks/data/`
2. Create `test/mocks/data/testdata.go`:
   - `ReadFile(testing, filePath) []byte` — file loader utility (copy from rover-server pattern)
3. Create fixture files:
   - `application.json` — `Application` CRD in namespace `poc--eni--hyperion`, name `my-app`
     - Spec: team=`hyperion`, teamEmail, zone ref, security (IpRestrictions)
     - Status: conditions with Ready=True
   - `apiExposure.json` — `ApiExposure` CRD in namespace `poc--eni--hyperion`
     - Label: `cp.ei.telekom.de/application=my-app`
     - Spec: apiBasePath, upstreams, visibility, approval, security
     - Status: conditions with Ready=True
   - `apiSubscription.json` — `ApiSubscription` CRD in namespace `poc--eni--hyperion`
     - Label: `cp.ei.telekom.de/application=my-app`
     - Spec: apiBasePath, zone ref, requestor ref, security
     - Status: approval ref, conditions with Ready=True
   - `zone.json` — `Zone` CRD
     - Status: Links.Url for gateway URL construction
   - `approval.json` — `Approval` CRD
     - Status: approval state (approved/pending/rejected)
   - `eventType.json` — `EventType` CRD (global, no namespace scoping)
     - Spec: eventType name, description
     - Status: conditions with Ready=True

**Deliverables:**
- All 6 fixture files with valid CRD structures
- File loader utility

### 🚧 Gate 2: Fixture Validation
- [ ] All JSON files are valid and can be unmarshaled into their respective CRD types
- [ ] Namespace patterns match test token configuration (`poc--eni--hyperion`)
- [ ] Cross-references between fixtures are consistent (e.g., subscription's zone ref matches zone fixture)

---

## Step 3: Mock Helpers — CRD Unmarshaling + Store Mocks

**Goal:** Create per-store mock helpers that load fixtures and configure mock expectations.

**Tasks:**
1. Create `test/mocks/mocks.go`:
   - `GetApplication(testing, filePath) *applicationv1.Application`
   - `GetApiExposure(testing, filePath) *apiv1.ApiExposure`
   - `GetApiSubscription(testing, filePath) *apiv1.ApiSubscription`
   - `GetZone(testing, filePath) *adminv1.Zone`
   - `GetApproval(testing, filePath) *approvalv1.Approval`
   - `GetEventType(testing, filePath) *eventv1.EventType`
2. Create per-store mock configuration files:
   - `mocks_Application.go`:
     - `NewApplicationStoreMock(t) store.ObjectStore[*applicationv1.Application]`
     - `ConfigureApplicationStoreMock(t, mock)` — Get returns fixture, List returns [fixture]
   - `mocks_ApiExposure.go`:
     - `NewAPIExposureStoreMock(t)` + `ConfigureAPIExposureStoreMock(t, mock)`
   - `mocks_ApiSubscription.go`:
     - `NewAPISubscriptionStoreMock(t)` + `ConfigureAPISubscriptionStoreMock(t, mock)`
   - `mocks_Zone.go`:
     - `NewZoneStoreMock(t)` + `ConfigureZoneStoreMock(t, mock)`
   - `mocks_Approval.go`:
     - `NewApprovalStoreMock(t)` + `ConfigureApprovalStoreMock(t, mock)`
   - `mocks_EventType.go`:
     - `NewEventTypeStoreMock(t)` + `ConfigureEventTypeStoreMock(t, mock)`
3. Create `NewEmptyStoreMock[T](t)` helper for unimplemented controllers:
   - Returns empty List responses
   - Get returns not-found error
4. Copy `MockObjectStore` type alias or import from `common-server/test/mocks/`

**Deliverables:**
- All mock helpers compile and are ready for use in test suite
- All mock expectations use `.Maybe()` for flexibility
- All mock expectations use `mock.Anything` for context parameters

### 🚧 Gate 3: Quality Check
- [ ] `go build ./...` passes (including test files with `_test.go` suffix)
- [ ] All mock helpers instantiate without errors
- [ ] Run **quality-check** skill

---

## Step 4: Test Suite Setup

**Goal:** Create the test suite infrastructure in `suite_controller_test.go`.

**Tasks:**
1. Create `internal/controller/suite_controller_test.go`:
   - `TestController(t *testing.T)` — Ginkgo bootstrap
   - `BeforeSuite`:
     - Initialize context
     - Create all mock stores via helpers from Step 3
     - Create security tokens: `teamToken`, `groupToken`, `adminToken`, `teamNoResourcesToken`
     - Create Fiber app via `cserver.NewApp()`
     - Create `server.Server` with controllers and mock stores
     - Call `s.RegisterRoutes(app)`
   - `AfterSuite`:
     - Cancel context
   - Helper functions:
     - `ExecuteRequest(request, bearerToken) (*http.Response, error)`
     - `ExpectStatusOk(response, err, matchers...)`
     - `ExpectStatusWithBody(response, err, statusCode, contentType, matchers...)`
     - `ExpectStatus(response, err, statusCode, contentType)`
2. Handle unimplemented controllers:
   - `EventExposures`, `EventSubscriptions`, `EventTypes` — create minimal stub controllers or use nil (if server allows)
   - If nil is not allowed, create stub implementations that return `501 Not Implemented`

**Deliverables:**
- Working test suite that compiles and runs (even with no tests yet)
- `go test ./internal/controller/...` passes with 0 tests

### 🚧 Gate 4: Suite Runs Successfully
- [ ] `go test ./internal/controller/... -v` passes (0 tests, no panics)
- [ ] Fiber app starts and registers all routes
- [ ] Security tokens are generated
- [ ] Run **quality-check** skill

---

## Step 5: Application Controller Tests

**Goal:** Implement tests for all 3 Application endpoints.

**Tasks:**
1. Create `internal/controller/application_test.go`:
   - `Describe("Application Controller", func() { ... })`
2. Test `GET /applications`:
   - `DescribeTable` with team/group/admin tokens → 200 OK with items
   - Test with `teamNoResourcesToken` → 200 OK with empty items
   - Verify `X-Total-Count` and `X-Result-Count` headers
   - Snapshot test for response body
3. Test `GET /applications/:applicationId`:
   - `DescribeTable` with team/group/admin tokens → 200 OK
   - Test not found → 404
   - Test forbidden (different team token) → 403
   - Snapshot test for response body
4. Test `GET /applications/:applicationId/status`:
   - `DescribeTable` with team/group/admin tokens → 200 OK
   - Test not found → 404
   - Test forbidden → 403
   - Snapshot test for response body

**Deliverables:**
- ≥12 passing tests (3 endpoints × 4 scope variations)
- Snapshot files generated
- All scopes covered

### 🚧 Gate 5: Quality Check + Test Coverage
- [ ] All Application controller tests pass: `go test ./internal/controller/... -v -run "Application"`
- [ ] Snapshot files generated at `__snapshots__/suite_controller_test.snap`
- [ ] Team, group, and admin scopes tested for each endpoint
- [ ] Error cases tested (404, 403)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: controller package coverage increasing)

---

## Step 6: ApiExposure Controller Tests

**Goal:** Implement tests for all 4 ApiExposure endpoints.

**Tasks:**
1. Create `internal/controller/apiexposure_test.go`:
   - `Describe("ApiExposure Controller", func() { ... })`
2. Test `GET /applications/:applicationId/apiexposures`:
   - `DescribeTable` with team/group/admin tokens → 200 OK with items
   - Test with `teamNoResourcesToken` → 403 (scoped under applicationId, not global)
   - Verify `X-Total-Count` and `X-Result-Count` headers
   - Snapshot test
3. Test `GET /applications/:applicationId/apiexposures/:apiExposureName`:
   - `DescribeTable` with team/group/admin tokens → 200 OK
   - Test not found → 404
   - Test forbidden → 403
   - Snapshot test
4. Test `GET /applications/:applicationId/apiexposures/:apiExposureName/status`:
   - Same pattern as Get
5. Test `GET /applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions`:
   - `DescribeTable` with team/group/admin tokens → 200 OK
   - Verify cross-namespace subscription resolution
   - Snapshot test

**Deliverables:**
- ≥16 passing tests (4 endpoints × 4 scope variations)
- Snapshot files updated

### 🚧 Gate 6: Quality Check + Test Coverage
- [ ] All ApiExposure controller tests pass
- [ ] Cross-namespace subscription resolution tested
- [ ] Team, group, and admin scopes tested for each endpoint
- [ ] Error cases tested (404, 403)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: increasing coverage)

---

## Step 7: ApiSubscription Controller Tests

**Goal:** Implement tests for all 3 ApiSubscription endpoints.

**Tasks:**
1. Create `internal/controller/apisubscription_test.go`:
   - `Describe("ApiSubscription Controller", func() { ... })`
2. Test `GET /applications/:applicationId/apisubscriptions`:
   - `DescribeTable` with team/group/admin tokens → 200 OK with items
   - Verify gateway URL construction in response
   - Verify approval status in response
   - Snapshot test
3. Test `GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName`:
   - `DescribeTable` with team/group/admin tokens → 200 OK
   - Verify cross-resource fields: `gatewayUrl`, `approval`, `team`
   - Test not found → 404
   - Test forbidden → 403
   - Snapshot test
4. Test `GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName/status`:
   - Same pattern as Get

**Deliverables:**
- ≥12 passing tests (3 endpoints × 4 scope variations)
- Cross-resource field resolution verified via snapshots

### 🚧 Gate 7: Quality Check + Test Coverage
- [ ] All ApiSubscription controller tests pass
- [ ] Cross-resource resolution tested (Zone → gatewayUrl, Approval → status)
- [ ] Team, group, and admin scopes tested for each endpoint
- [ ] Error cases tested (404, 403)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: ≥80% for controller package)

---

## Step 8: Deprecated Endpoint Tests + Cleanup

**Goal:** Test all deprecated endpoints and finalize the test suite.

**Tasks:**
1. Create `internal/controller/deprecated_test.go`:
   - `Describe("Deprecated endpoints", func() { ... })`
   - `DescribeTable` with all 10 deprecated endpoint method+path combinations
   - Verify each returns 410 Gone with `application/problem+json` content type
2. Add SPDX license headers to all new test files and mock files
3. Run `pre-commit run --all-files` and fix any issues
4. Verify snapshot files are committed and up to date
5. Run full test suite: `go test ./... -v`

**Deliverables:**
- 10 deprecated endpoint tests passing
- All license headers in place
- Full test suite green

### 🚧 Gate 8: Final Review
- [ ] Full test suite passes: `go test ./... -v`
- [ ] Total tests: ≥53 (minimum from coverage matrix)
- [ ] All 3 security scopes covered for all scoped endpoints
- [ ] All deprecated endpoints return 410
- [ ] Snapshot files committed
- [ ] `pre-commit run --all-files` passes
- [ ] Run **quality-check** skill (final pass)
- [ ] Run **test-coverage** skill (final: ≥80% for controller package)

---

## Summary

| Step | Description | Depends On | Gate |
|------|-------------|------------|------|
| 1 | Add test dependencies | — | Build passes |
| 2 | Test data fixtures | Step 1 | Fixtures validate |
| 3 | Mock helpers | Steps 1, 2 | Build passes |
| 4 | Test suite setup | Step 3 | Suite runs (0 tests) |
| 5 | Application tests | Step 4 | ≥12 tests pass, scopes covered |
| 6 | ApiExposure tests | Step 4 | ≥16 tests pass, scopes covered |
| 7 | ApiSubscription tests | Step 4 | ≥12 tests pass, scopes covered |
| 8 | Deprecated tests + cleanup | Steps 5–7 | ≥53 total tests, ≥80% coverage |

**Parallelism opportunities:** Steps 5, 6, and 7 can be developed in parallel after Step 4 is complete (they are independent test files sharing the same suite setup).

**Estimated effort:** Medium — ~15 new files (6 fixtures, 7 mock helpers, 5 test files), following established rover-server patterns exactly.

### Test Count Estimate

| Test File | Estimated Tests |
|-----------|----------------|
| `application_test.go` | 12–15 |
| `apiexposure_test.go` | 16–20 |
| `apisubscription_test.go` | 12–15 |
| `deprecated_test.go` | 10 |
| **Total** | **50–60** |
