<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Testing Concept — Design Document

> **Version:** 1.0  
> **Date:** 2025-03-30  
> **Status:** Draft  
> **Component:** `spy-server/`  
> **Reference Implementation:** `rover-server/internal/controller/*_test.go`

---

## Table of Contents

1. [Overview](#1-overview)
2. [Scope](#2-scope)
3. [Framework Choices](#3-framework-choices)
   - [3.1 Ginkgo v2 + Gomega](#31-ginkgo-v2--gomega)
   - [3.2 Snapshot Testing — go-snaps](#32-snapshot-testing--go-snaps)
   - [3.3 Table-Driven Tests — Ginkgo DescribeTable](#33-table-driven-tests--ginkgo-describetable)
4. [Test Architecture](#4-test-architecture)
   - [4.1 HTTP-Through Testing](#41-http-through-testing)
   - [4.2 Test Suite Setup](#42-test-suite-setup)
   - [4.3 Security Token Generation](#43-security-token-generation)
   - [4.4 Helper Functions](#44-helper-functions)
5. [Mock Strategy](#5-mock-strategy)
   - [5.1 MockObjectStore](#51-mockobjectstore)
   - [5.2 Per-Store Mock Helpers](#52-per-store-mock-helpers)
   - [5.3 Test Data Fixtures](#53-test-data-fixtures)
6. [Security Scope Coverage Matrix](#6-security-scope-coverage-matrix)
7. [Endpoint Test Specification](#7-endpoint-test-specification)
   - [7.1 Application Endpoints](#71-application-endpoints)
   - [7.2 ApiExposure Endpoints](#72-apiexposure-endpoints)
   - [7.3 ApiSubscription Endpoints](#73-apisubscription-endpoints)
   - [7.4 EventType Endpoints](#74-eventtype-endpoints)
   - [7.5 Deprecated Endpoints](#75-deprecated-endpoints)
8. [Directory Structure](#8-directory-structure)
9. [Potential Issues & Risks](#9-potential-issues--risks)
10. [Decision Log](#10-decision-log)

---

## 1. Overview

The **spy-server** currently has **zero test files**. This document defines the testing concept for a greenfield testing effort, covering all 11 active GET endpoints, 10 deprecated endpoints, and all 3 security scopes (team, group, admin).

The testing approach follows the patterns established by `rover-server/internal/controller/*_test.go`, adapted for spy-server's read-only, nested-resource architecture.

### Key Facts

| Aspect | Value |
|--------|-------|
| Current test coverage | 0% (no test files exist) |
| Target test coverage | ≥80% for controller package |
| Test framework | Ginkgo v2 + Gomega |
| Snapshot library | go-snaps |
| Mock library | testify/mock via MockObjectStore (mockery-generated) |
| Active endpoints to test | 11 GET endpoints |
| Deprecated endpoints to test | 10 POST/PUT/DELETE endpoints |
| Security scopes to cover | 3 (team, group, admin) |
| Minimum tests per endpoint | 1 per scope (team, group, admin) |

---

## 2. Scope

### In Scope

- **Controller integration tests** — HTTP-through tests exercising the full middleware chain (JWT, BusinessContext, CheckAccess, OpenAPI validation, controller, mapper, mock store)
- **All 11 active GET endpoints** — at least one test per endpoint per security scope
- **All 10 deprecated endpoints** — verify 410 Gone responses
- **Security scope coverage** — team, group/hub, and admin access levels
- **Error cases** — not found, forbidden, invalid applicationId format
- **Snapshot testing** — response body verification via go-snaps
- **Mock store infrastructure** — per-store mock helpers following rover-server patterns

### Out of Scope

- **Unit tests for individual mappers** — deferred to a future iteration (mappers are tested transitively through controller tests)
- **Unit tests for pagination utilities** — deferred (tested transitively through list endpoint tests)
- **Performance/load testing** — separate concern
- **EventExposure / EventSubscription controllers** — not yet implemented (interfaces defined, no implementation)
- **End-to-end tests against a real cluster** — separate concern

---

## 3. Framework Choices

### 3.1 Ginkgo v2 + Gomega

**Decision:** Use **Ginkgo v2** as the test runner and **Gomega** as the assertion library.

**Rationale:**
- Consistent with rover-server's existing test suite
- BDD-style `Describe`/`Context`/`It` blocks provide readable, well-organized test structure
- Ginkgo's `BeforeSuite`/`AfterSuite` lifecycle hooks are ideal for one-time test setup (mock stores, Fiber app, security tokens)
- Gomega's fluent matchers (`Expect(x).To(Equal(y))`) are expressive and produce clear failure messages
- `GinkgoT()` bridges Ginkgo with testify/mock for mock assertions

**Versions (matching rover-server):**

```
github.com/onsi/ginkgo/v2 v2.28.1
github.com/onsi/gomega v1.39.1
```

### 3.2 Snapshot Testing — go-snaps

**Decision:** Use **go-snaps** for response body verification in controller tests.

**Evaluation:**

| Aspect | Pros | Cons |
|--------|------|------|
| **Accuracy** | Captures full JSON response → catches unintended field changes | Must update snapshots when response format intentionally changes |
| **Maintenance** | Auto-generates `.snap` files → no manual expected-value maintenance | Snapshot files grow large if many endpoints are tested |
| **Dynamic fields** | `match.Any("fieldName")` handles timestamps, IDs | Requires explicit exclusion of dynamic fields |
| **Developer experience** | `go-snaps -u` updates all snapshots | New developers must understand snapshot workflow |
| **Proven in project** | Already used successfully in rover-server | — |

**Conclusion:** go-snaps is **recommended** for spy-server controller tests. It is proven within the project (rover-server uses it), reduces boilerplate compared to manual JSON assertions, and the `match.Any()` escape hatch handles dynamic fields cleanly.

**Version:**

```
github.com/gkampitakis/go-snaps v0.5.21
```

**Usage pattern:**

```go
func expectResponseWithBody(response *http.Response, matchers ...match.JSONMatcher) {
    b, err := io.ReadAll(response.Body)
    Expect(err).ToNot(HaveOccurred())
    snaps.MatchJSON(GinkgoT(), string(b), matchers...)
}
```

Snapshot files are stored at `internal/controller/__snapshots__/suite_controller_test.snap` (auto-generated by go-snaps).

### 3.3 Table-Driven Tests — Ginkgo DescribeTable

**Decision:** Use Ginkgo's `DescribeTable` / `Entry` for tests that verify the same behavior across multiple security scopes.

**Rationale:**
- Each endpoint must be tested with team, group, and admin tokens
- Table-driven tests reduce duplication while keeping each scope explicit
- Clear per-entry labels make test output readable

**Where to use:**
- **Security scope coverage** — same endpoint, different tokens → same expected behavior (200 OK)
- **Deprecated endpoint verification** — same 410 behavior across multiple routes
- **Error cases** — same error pattern across multiple endpoints (e.g., invalid applicationId)

**Where NOT to use:**
- Tests with significantly different setup or assertions per case
- Tests with unique mock configurations per case

**Example pattern:**

```go
DescribeTable("should be accessible with all security scopes",
    func(token string, description string) {
        req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
        resp, err := ExecuteRequest(req, token)
        ExpectStatusOk(resp, err)
    },
    Entry("team scope", teamToken, "team-level access"),
    Entry("group scope", groupToken, "group-level access"),
    Entry("admin scope", adminToken, "admin-level access"),
)
```

---

## 4. Test Architecture

### 4.1 HTTP-Through Testing

Tests exercise the **full HTTP middleware chain** by sending requests through Fiber's built-in test helper:

```
httptest.NewRequest → app.Test(request, -1) → http.Response
         │
         ▼
   ┌─────────────┐
   │ Fiber App    │
   │  ├─ JWT mock │ ← securitymock token
   │  ├─ BusinessContext
   │  ├─ CheckAccess
   │  ├─ OpenAPI validator
   │  ├─ Controller
   │  ├─ Mapper
   │  └─ Mock Store ← testify/mock
   └─────────────┘
```

This approach validates:
- Security middleware correctly grants/denies access per scope
- OpenAPI request validation rejects malformed requests
- Controller correctly orchestrates store calls and mapping
- Response JSON structure matches the OpenAPI spec
- HTTP headers (Content-Type, X-Total-Count, X-Result-Count) are set correctly

### 4.2 Test Suite Setup

A single `suite_controller_test.go` file sets up the shared test infrastructure in `BeforeSuite`:

```go
package controller

var (
    ctx    context.Context
    cancel context.CancelFunc
    app    *fiber.App
    stores *sstore.Stores

    teamToken  string  // tardis:team:all — prefix: poc--eni--hyperion--
    groupToken string  // tardis:group:all — prefix: poc--eni--
    adminToken string  // tardis:admin:all — prefix: poc--
    teamNoResourcesToken string  // different team, no matching data
)

func TestController(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.TODO())

    // 1. Create mock stores
    stores = &sstore.Stores{
        ApplicationStore:     mocks.NewApplicationStoreMock(GinkgoT()),
        APIExposureStore:     mocks.NewAPIExposureStoreMock(GinkgoT()),
        APISubscriptionStore: mocks.NewAPISubscriptionStoreMock(GinkgoT()),
        ZoneStore:            mocks.NewZoneStoreMock(GinkgoT()),
        ApprovalStore:        mocks.NewApprovalStoreMock(GinkgoT()),
        EventTypeStore:       mocks.NewEventTypeStoreMock(GinkgoT()),
        EventExposureStore:   mocks.NewEmptyStoreMock[*eventv1.EventExposure](GinkgoT()),
        EventSubscriptionStore: mocks.NewEmptyStoreMock[*eventv1.EventSubscription](GinkgoT()),
    }

    // 2. Create security tokens
    teamToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:team:all"})
    groupToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:group:all"})
    adminToken = securitymock.NewMockAccessToken("poc", "eni", "hyperion", []string{"tardis:admin:all"})
    teamNoResourcesToken = securitymock.NewMockAccessToken("poc", "eni", "nohyper", []string{"tardis:team:all"})

    // 3. Create Fiber app and register routes
    app = cserver.NewApp()
    s := server.Server{
        Config:           &config.ServerConfig{},
        Log:              log.Log,
        Applications:     NewApplicationController(stores),
        ApiExposures:     NewApiExposureController(stores),
        ApiSubscriptions: NewApiSubscriptionController(stores),
        // EventExposures, EventSubscriptions, EventTypes — wired when implemented
    }
    s.RegisterRoutes(app)
})
```

### 4.3 Security Token Generation

Three tokens are generated per test suite, plus a "no resources" token:

| Token | Env | Group | Team | Scope | Purpose |
|-------|-----|-------|------|-------|---------|
| `teamToken` | `poc` | `eni` | `hyperion` | `tardis:team:all` | Access resources in `poc--eni--hyperion` namespace |
| `groupToken` | `poc` | `eni` | `hyperion` | `tardis:group:all` | Access any resource in `poc--eni--*` namespaces |
| `adminToken` | `poc` | `eni` | `hyperion` | `tardis:admin:all` | Access any resource in `poc--*` namespaces |
| `teamNoResourcesToken` | `poc` | `eni` | `nohyper` | `tardis:team:all` | No matching mock data → tests empty results |

The mock data fixtures use the namespace `poc--eni--hyperion` consistently, so all three tokens can access the same data (with different prefix granularity).

### 4.4 Helper Functions

Reusable helpers following rover-server's pattern:

```go
func ExecuteRequest(request *http.Request, bearerToken string) (*http.Response, error)
func ExpectStatusOk(response *http.Response, err error, matchers ...match.JSONMatcher)
func ExpectStatusWithBody(response *http.Response, err error, statusCode int, contentType string, matchers ...match.JSONMatcher)
func ExpectStatus(response *http.Response, err error, statusCode int, contentType string)
```

---

## 5. Mock Strategy

### 5.1 MockObjectStore

The `MockObjectStore[T]` from `common-server/test/mocks/mock_ObjectStore.go` is a mockery-generated generic mock implementing `store.ObjectStore[T]`. It uses testify/mock with the `.EXPECT()` fluent API.

**Instantiation:** `NewMockObjectStore[T](GinkgoT())` where `GinkgoT()` bridges Ginkgo with testify.

**Key mock methods used by spy-server controllers:**

| Method | Signature | Used By |
|--------|-----------|---------|
| `Get` | `Get(ctx, namespace, name) → (T, error)` | All single-resource endpoints |
| `List` | `List(ctx, opts) → (*ListResponse[T], error)` | All list endpoints (via `FetchAll`) |

Other methods (`CreateOrReplace`, `Delete`, `Patch`, `Ready`, `Info`) are **not needed** since spy-server is read-only.

### 5.2 Per-Store Mock Helpers

Each store type gets a dedicated mock helper file following rover-server's pattern:

```go
// test/mocks/mocks_Application.go
func NewApplicationStoreMock(testing ginkgo.FullGinkgoTInterface) store.ObjectStore[*applicationv1.Application] {
    mockStore := NewMockObjectStore[*applicationv1.Application](testing)
    ConfigureApplicationStoreMock(testing, mockStore)
    return mockStore
}

func ConfigureApplicationStoreMock(testing ginkgo.FullGinkgoTInterface, mockedStore *MockObjectStore[*applicationv1.Application]) {
    application := GetApplication(testing, applicationFileName)

    mockedStore.EXPECT().Get(
        mock.AnythingOfType("*context.valueCtx"),
        mock.AnythingOfType("string"),
        mock.Anything,
    ).Return(application, nil).Maybe()

    mockedStore.EXPECT().List(
        mock.AnythingOfType("*context.valueCtx"),
        mock.Anything,
    ).Return(
        &store.ListResponse[*applicationv1.Application]{
            Items: []*applicationv1.Application{application},
        }, nil).Maybe()
}
```

**Mock helpers needed for spy-server:**

| File | Store Type | Notes |
|------|-----------|-------|
| `mocks_Application.go` | `ApplicationStore` | Get + List |
| `mocks_ApiExposure.go` | `APIExposureStore` | Get + List |
| `mocks_ApiSubscription.go` | `APISubscriptionStore` | Get + List |
| `mocks_Zone.go` | `ZoneStore` | Get only (used for gateway URL resolution) |
| `mocks_Approval.go` | `ApprovalStore` | Get only (used for subscription approval status) |
| `mocks_EventType.go` | `EventTypeStore` | Get + List |

**Stores not yet needing dedicated mocks** (controllers not implemented):
- `EventExposureStore` — use `NewEmptyStoreMock[T]()` returning empty lists
- `EventSubscriptionStore` — use `NewEmptyStoreMock[T]()` returning empty lists

### 5.3 Test Data Fixtures

JSON fixture files containing valid CRD objects, stored at `test/mocks/data/`:

| File | CRD Type | Notes |
|------|----------|-------|
| `application.json` | `Application` | Namespace: `poc--eni--hyperion`, name: `my-app` |
| `apiExposure.json` | `ApiExposure` | Label: `cp.ei.telekom.de/application=my-app` |
| `apiSubscription.json` | `ApiSubscription` | With zone ref, approval ref, requestor ref |
| `zone.json` | `Zone` | With `Status.Links.Url` for gateway URL construction |
| `approval.json` | `Approval` | With approval status |
| `eventType.json` | `EventType` | Global resource (no namespace scoping) |

**Fixture loading pattern** (from rover-server):

```go
// test/mocks/data/testdata.go
func ReadFile(testing ginkgo.FullGinkgoTInterface, filePath string) []byte {
    testDataDir, _ := filepath.Abs(getCurrentFileDir())
    file, err := os.ReadFile(filepath.Join(testDataDir, filePath))
    require.NoError(testing, err)
    return file
}
```

---

## 6. Security Scope Coverage Matrix

Every active endpoint must be tested with **at least one test per security scope**. The matrix below shows the minimum test coverage:

### Application Endpoints

| Endpoint | Team | Group | Admin | No-Access |
|----------|------|-------|-------|-----------|
| `GET /applications` | ✅ List filtered by prefix | ✅ List filtered by prefix | ✅ List filtered by prefix | ✅ Empty list |
| `GET /applications/:id` | ✅ Own app | ✅ Any group app | ✅ Any env app | ✅ 403 Forbidden |
| `GET /applications/:id/status` | ✅ Own app | ✅ Any group app | ✅ Any env app | ✅ 403 Forbidden |

### ApiExposure Endpoints

| Endpoint | Team | Group | Admin | No-Access |
|----------|------|-------|-------|-----------|
| `GET .../apiexposures` | ✅ List | ✅ List | ✅ List | ✅ 403 |
| `GET .../apiexposures/:name` | ✅ Get | ✅ Get | ✅ Get | ✅ 403 |
| `GET .../apiexposures/:name/status` | ✅ Get | ✅ Get | ✅ Get | ✅ 403 |
| `GET .../apiexposures/:name/apisubscriptions` | ✅ Get | ✅ Get | ✅ Get | ✅ 403 |

### ApiSubscription Endpoints

| Endpoint | Team | Group | Admin | No-Access |
|----------|------|-------|-------|-----------|
| `GET .../apisubscriptions` | ✅ List | ✅ List | ✅ List | ✅ 403 |
| `GET .../apisubscriptions/:name` | ✅ Get | ✅ Get | ✅ Get | ✅ 403 |
| `GET .../apisubscriptions/:name/status` | ✅ Get | ✅ Get | ✅ Get | ✅ 403 |

### EventType Endpoints (No `checkAccess` — globally accessible)

| Endpoint | Any Token |
|----------|-----------|
| `GET /eventtypes` | ✅ List |
| `GET /eventtypes/:name` | ✅ Get |
| `GET /eventtypes/:name/status` | ✅ Get |

> **Note:** EventType endpoints do NOT use `checkAccess` middleware — they are globally accessible. Tests only need to verify they work with any valid token.

### Deprecated Endpoints

| Endpoint | Any Token |
|----------|-----------|
| All 10 deprecated POST/PUT/DELETE endpoints | ✅ 410 Gone |

### Total Minimum Tests

| Category | Endpoints | Tests per endpoint | Total |
|----------|-----------|-------------------|-------|
| Application (scoped) | 3 | 4 (team + group + admin + no-access) | 12 |
| ApiExposure (scoped) | 4 | 4 | 16 |
| ApiSubscription (scoped) | 3 | 4 | 12 |
| EventType (global) | 3 | 1 | 3 |
| Deprecated | 10 | 1 | 10 |
| **Total minimum** | **23** | — | **53** |

Additional tests for error cases (not found, invalid format) will increase the total.

---

## 7. Endpoint Test Specification

### 7.1 Application Endpoints

#### `GET /applications` — List All Applications

```go
Context("GetAll applications", func() {
    DescribeTable("should return applications for all scopes",
        func(token string) {
            req := httptest.NewRequest(http.MethodGet, "/applications", nil)
            resp, err := ExecuteRequest(req, token)
            ExpectStatusOk(resp, err)
        },
        Entry("team scope", teamToken),
        Entry("group scope", groupToken),
        Entry("admin scope", adminToken),
    )

    It("should return empty list for team with no resources", func() {
        req := httptest.NewRequest(http.MethodGet, "/applications", nil)
        resp, err := ExecuteRequest(req, teamNoResourcesToken)
        ExpectStatusOk(resp, err) // 200 with empty items
    })
})
```

#### `GET /applications/:applicationId` — Get Single Application

```go
Context("Get application", func() {
    DescribeTable("should get application for all scopes",
        func(token string) {
            req := httptest.NewRequest(http.MethodGet, "/applications/eni--hyperion--my-app", nil)
            resp, err := ExecuteRequest(req, token)
            ExpectStatusOk(resp, err)
        },
        Entry("team scope", teamToken),
        Entry("group scope", groupToken),
        Entry("admin scope", adminToken),
    )

    It("should return 404 for non-existent application", func() { ... })
    It("should return 403 for application in different team", func() { ... })
})
```

#### `GET /applications/:applicationId/status` — Get Application Status

Same pattern as Get, testing all scopes + error cases.

### 7.2 ApiExposure Endpoints

All 4 endpoints follow the same pattern. The `GetSubscriptions` endpoint is additionally tested for correct cross-namespace subscription resolution.

### 7.3 ApiSubscription Endpoints

All 3 endpoints follow the same pattern. Tests additionally verify:
- Gateway URL construction from Zone store lookup
- Approval status from Approval store lookup
- Graceful handling when Zone/Approval/Application are not found

### 7.4 EventType Endpoints

Simpler tests — no security scope variations needed since `checkAccess` is not applied.

### 7.5 Deprecated Endpoints

```go
Context("Deprecated endpoints", func() {
    DescribeTable("should return 410 Gone",
        func(method, path string) {
            req := httptest.NewRequest(method, path, nil)
            resp, err := ExecuteRequest(req, groupToken)
            ExpectStatus(resp, err, http.StatusGone, "application/problem+json")
        },
        Entry("POST /applications", http.MethodPost, "/applications"),
        Entry("PUT /applications/:id", http.MethodPut, "/applications/eni--hyperion--my-app"),
        Entry("DELETE /applications/:id", http.MethodDelete, "/applications/eni--hyperion--my-app"),
        // ... remaining deprecated endpoints
    )
})
```

---

## 8. Directory Structure

```
spy-server/
├── internal/
│   └── controller/
│       ├── suite_controller_test.go          # Test suite setup (BeforeSuite, helpers)
│       ├── application_test.go               # Application endpoint tests
│       ├── apiexposure_test.go               # ApiExposure endpoint tests
│       ├── apisubscription_test.go           # ApiSubscription endpoint tests
│       ├── eventtype_test.go                 # EventType endpoint tests (when implemented)
│       ├── deprecated_test.go                # Deprecated endpoint tests
│       └── __snapshots__/
│           └── suite_controller_test.snap    # Auto-generated by go-snaps
└── test/
    └── mocks/
        ├── mocks.go                          # CRD type unmarshaling helpers
        ├── mocks_Application.go              # Application store mock configuration
        ├── mocks_ApiExposure.go              # ApiExposure store mock configuration
        ├── mocks_ApiSubscription.go          # ApiSubscription store mock configuration
        ├── mocks_Zone.go                     # Zone store mock configuration
        ├── mocks_Approval.go                 # Approval store mock configuration
        ├── mocks_EventType.go                # EventType store mock configuration
        └── data/
            ├── testdata.go                   # File loader utility
            ├── application.json              # Application CRD fixture
            ├── apiExposure.json              # ApiExposure CRD fixture
            ├── apiSubscription.json          # ApiSubscription CRD fixture
            ├── zone.json                     # Zone CRD fixture
            ├── approval.json                 # Approval CRD fixture
            └── eventType.json                # EventType CRD fixture
```

---

## 9. Potential Issues & Risks

### 🟡 Important

#### Issue 1: EventExposure / EventSubscription Controllers Not Implemented
**Problem:** The `EventExposureController`, `EventSubscriptionController`, and `EventTypeController` interfaces are defined in `server.go`, but no implementations exist.  
**Impact:** Tests cannot be written for these endpoints until controllers are implemented. The `Server` struct requires all controllers to be set.  
**Mitigation:** Use stub implementations that panic or return `501 Not Implemented` for unimplemented controllers. Test only the implemented controllers (Application, ApiExposure, ApiSubscription) initially.

#### Issue 2: OpenAPI Validator May Block Test Requests
**Problem:** The OpenAPI request validator middleware validates all incoming requests against the embedded spec. Test requests must conform to the spec.  
**Impact:** Tests may fail due to validation errors unrelated to the controller logic being tested.  
**Mitigation:** Ensure all test requests use valid paths, query parameters, and content types as defined in the OpenAPI spec. This is actually a benefit — it catches spec-violating behavior early.

#### Issue 3: Mock Context Type Matching
**Problem:** Store mock expectations use `mock.AnythingOfType("*context.valueCtx")` for context matching. The actual context type passed through Fiber's middleware chain may differ (e.g., `*context.cancelCtx`, `*context.timerCtx`).  
**Impact:** Mock expectations may not match, causing unexpected failures.  
**Mitigation:** Use `mock.Anything` for context parameters instead of specific type matching. This is a pragmatic choice — we don't need to assert on the context type.

### 🟢 Minor

#### Issue 4: Snapshot File Size
**Problem:** With 53+ tests generating snapshots, the `.snap` file may become large.  
**Impact:** Harder to review in PRs.  
**Mitigation:** Acceptable trade-off. Snapshot diffs in PRs are useful for catching unintended response changes. The `.snap` file is auto-generated and can be updated with `go-snaps -u`.

#### Issue 5: go.mod Dependency Additions
**Problem:** spy-server's `go.mod` does not currently include `ginkgo`, `gomega`, or `go-snaps`.  
**Impact:** Dependencies must be added before any tests can compile.  
**Mitigation:** Run `go get` for each dependency in the first implementation step.

---

## 10. Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Use Ginkgo v2 + Gomega | Consistent with rover-server; BDD-style tests are readable and well-organized |
| D2 | Use go-snaps for snapshot testing | Proven in rover-server; reduces boilerplate; catches unintended response changes |
| D3 | Use `DescribeTable` for security scope coverage | Reduces duplication; each scope is a separate `Entry` with clear labeling |
| D4 | HTTP-through testing (not unit testing controllers) | Validates full middleware chain including security, OpenAPI validation, and response formatting |
| D5 | One test file per resource type | Keeps files focused and manageable; matches rover-server pattern |
| D6 | Shared `suite_controller_test.go` for setup | One-time setup of app, stores, tokens; shared across all test files in the package |
| D7 | Mock data from JSON fixtures | Real CRD structures; easy to inspect and update; same pattern as rover-server |
| D8 | `.Maybe()` on all mock expectations | Tests don't need to assert store call counts — focus is on HTTP response correctness |
| D9 | Start with Application, ApiExposure, ApiSubscription | These controllers are implemented; EventExposure/EventSubscription/EventType deferred |
| D10 | Use `mock.Anything` for context parameters | Avoids brittle type assertions on context; context is an implementation detail |
| D11 | Number the testing docs as `002_testing` | Next available number after `001_application` |
