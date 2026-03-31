<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# spy-server Implementation Plan

> **Depends on:** [design.md](./design.md)  
> **Estimated steps:** 10  
> **Status:** Not started

---

## Step 0: Project Scaffolding

**Goal:** Create the spy-server directory structure, `go.mod`, `Makefile`, and code generation tooling.

**Tasks:**
1. Create directory structure:
   ```
   spy-server/
   ├── api/
   │   └── openapi.yaml          ← move from spy-server/openapi.yaml
   ├── cmd/
   │   └── main.go               ← minimal main (panic-on-start placeholder)
   ├── internal/
   │   ├── api/                   ← generated code target
   │   ├── config/
   │   ├── server/
   │   ├── controller/
   │   └── mapper/
   │       ├── apiexposure/out/
   │       ├── apisubscription/out/
   │       └── status/
   ├── pkg/
   │   ├── log/
   │   └── store/
   ├── tools/
   │   ├── generate.go
   │   └── server.yaml
   ├── Makefile
   └── go.mod
   ```
2. Initialize `go.mod` with module path `github.com/telekom/controlplane/spy-server`
3. Copy and adapt `Makefile` from rover-server (remove write-related targets)
4. Create `tools/server.yaml` (oapi-codegen config — types + embedded spec only)
5. Create `tools/generate.go` with `//go:generate` directive
6. Run `go generate ./...` to produce `internal/api/server.gen.go`
7. Verify generated types compile: `go build ./...`

**Deliverables:**
- Compiling Go module with generated OpenAPI types
- All directories in place
- Working `make generate` target

### 🚧 Gate 0: Quality Check
- [ ] `go build ./...` passes
- [ ] Generated types include: `ApiExposureResponse`, `ApiSubscriptionResponse`, `ApiExposureListResponse`, `ApiSubscriptionListResponse`, `Paging`, `Links`, `ResourceStatusResponse`
- [ ] `make generate` is idempotent (running twice produces same output)
- [ ] Run **quality-check** skill

---

## Step 1: Store Layer

**Goal:** Implement the store dependency container with all required ObjectStores.

**Tasks:**
1. Create `pkg/store/stores.go`:
   - `Stores` struct with 5 typed ObjectStore fields (see design.md §4.5.1)
   - `NewStores(ctx, cfg)` constructor
   - `NewOrDie[T]()` helper (copy pattern from rover-server)
2. Import CRD types:
   - `apiv1` → ApiExposure, ApiSubscription
   - `applicationv1` → Application
   - `adminv1` → Zone
   - `approvalv1` → Approval
3. Update `go.mod` with local `replace` directives matching rover-server's pattern
4. Verify compilation

**Deliverables:**
- `pkg/store/stores.go` with all 5 stores
- `go.mod` updated with correct dependencies

### 🚧 Gate 1: Quality Check
- [ ] `go build ./...` passes
- [ ] All 5 store types resolve correctly
- [ ] Run **quality-check** skill

---

## Step 2: Configuration

**Goal:** Implement server configuration loading.

**Tasks:**
1. Create `internal/config/config.go`:
   - `ServerConfig` struct (Address, Security settings)
   - `LoadConfig()` using Viper (same pattern as rover-server)
2. Create `pkg/log/log.go`:
   - Logger initialization (copy from rover-server)
3. Ensure config includes security settings:
   - `Security.TrustedIssuers`
   - `Security.LMS.BasePath`
   - `Security.ScopePrefix` (default: `"tardis:"`)
   - `Security.DefaultScope` (default: `"tardis:user:read"`)

**Deliverables:**
- Working config loading
- Logger initialization

### 🚧 Gate 2: Quality Check
- [ ] `go build ./...` passes
- [ ] Run **quality-check** skill

---

## Step 3: Mapper Layer — Utilities & Status

**Goal:** Implement shared mapper utilities and the status mapper.

**Tasks:**
1. Create `internal/mapper/util.go`:
   - `ParseApplicationId(applicationId string) (group, team, appName string, err error)`
   - `BuildNamespace(env, group, team string) string`
   - `BuildApplicationLabel(appName string) store.Filter`
2. Create `internal/mapper/status/response.go`:
   - `MapStatusResponse(conditions []metav1.Condition) api.ResourceStatusResponse`
   - `mapProcessingState(conditions) api.ProcessingState`
   - `mapOverallStatus(conditions) api.OverallStatus`
   - Reuse/adapt logic from `rover-server/internal/mapper/status/`

**Deliverables:**
- Utility functions for applicationId parsing
- Status mapper producing `ResourceStatusResponse`

### 🚧 Gate 3: Quality Check + Test Coverage
- [ ] Unit tests for `ParseApplicationId` (valid format, invalid format, edge cases)
- [ ] Unit tests for status mapper (Ready condition, Failed condition, multiple conditions)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for mapper package)

---

## Step 4: Mapper Layer — ApiExposure Out-Mapper

**Goal:** Implement the CRD → API response mapper for ApiExposure.

**Tasks:**
1. Create `internal/mapper/apiexposure/out/apiexposure.go`:
   - `MapApiExposure(obj *apiv1.ApiExposure) api.ApiExposureResponse`
   - Map all spec fields: apiBasePath, upstreams, visibility, approval, security
   - Handle the `variant` field: default to `"DEFAULT"` (see design.md Issue 2)
2. Create helper functions for nested type mapping:
   - `mapUpstreams([]apiv1.Upstream) []api.Upstream`
   - `mapVisibility(apiv1.Visibility) api.Visibility`
   - `mapApprovalConfig(apiv1.Approval) api.ApprovalConfig`
   - `mapSecurity(apiv1.Security) api.ExposureSecurity`

**Deliverables:**
- Complete ApiExposure out-mapper
- All nested types mapped

### 🚧 Gate 4: Quality Check + Test Coverage
- [ ] Unit tests with sample ApiExposure CRD objects → expected API responses
- [ ] Test `variant` defaults to `"DEFAULT"`
- [ ] Test nil/empty nested fields don't cause panics
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for apiexposure mapper)

---

## Step 5: Mapper Layer — ApiSubscription Out-Mapper

**Goal:** Implement the CRD → API response mapper for ApiSubscription, including cross-resource context.

**Tasks:**
1. Create `internal/mapper/apisubscription/out/apisubscription.go`:
   - Define `SubscriptionMapContext` struct
   - `MapApiSubscription(mctx SubscriptionMapContext) api.ApiSubscriptionResponse`
   - Map core fields: name, apiBasePath, security
   - Map cross-resource fields: application, team, gatewayUrl, approval, failover
2. Handle nil cross-resource data gracefully (Application/Zone/Approval may not exist)
3. Implement `mapApprovalStatus(approval *approvalv1.Approval) api.ApprovalStatus`
4. Implement `buildGatewayUrl(zone *adminv1.Zone, basePath string) string`

**Deliverables:**
- Complete ApiSubscription out-mapper with cross-resource support
- Graceful nil handling for all optional cross-resource data

### 🚧 Gate 5: Quality Check + Test Coverage
- [ ] Unit tests with full SubscriptionMapContext → expected response
- [ ] Unit tests with nil Application, nil Zone, nil Approval
- [ ] Test gatewayUrl construction: Zone.Status.Links.Url + ApiBasePath
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for apisubscription mapper)

---

## Step 6: Controller Layer — Pagination Adapter

**Goal:** Implement the in-memory pagination adapter that bridges cursor-based stores to offset/limit responses.

**Tasks:**
1. Create `internal/controller/util.go`:
   - `fetchAll[T store.Object](ctx, store, opts) ([]T, error)` — drain cursor pagination
   - `PaginatedResult[T]` struct with Items, Paging, Links
   - `paginate[T any](items []T, offset, limit *int, baseUrl string) PaginatedResult[T]`
   - `buildLinks(offset, limit, total int, baseUrl string) api.Links` — HATEOAS link builder
   - `extractCursor(nextUrl string) string` — parse cursor from next URL
2. Ensure `fetchAll` has a safety limit (e.g., max 10,000 items) to prevent unbounded memory growth

**Deliverables:**
- Reusable pagination adapter
- HATEOAS link builder generating first/self/last/prev/next links

### 🚧 Gate 6: Quality Check + Test Coverage
- [ ] Unit tests for `paginate`: first page, middle page, last page, empty list, offset beyond total
- [ ] Unit tests for `buildLinks`: verify all 5 link types
- [ ] Unit tests for `fetchAll` with mock store returning multiple pages
- [ ] Test safety limit prevents infinite loop
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 90% for pagination utilities)

---

## Step 7: Controller Layer — ApiExposure & ApiSubscription Controllers

**Goal:** Implement the controller interfaces for both resource types.

**Tasks:**
1. Create `internal/controller/apiexposure.go`:
   - `NewApiExposureController(stores) ApiExposureController`
   - `Get(ctx, applicationId, name)` — resolve namespace, store.Get, map response
   - `GetAll(ctx, applicationId, params)` — resolve namespace, label filter, fetchAll, paginate, map
   - `GetStatus(ctx, applicationId, name)` — store.Get, map status
   - `GetSubscriptions(ctx, applicationId, name)` — get exposure, filter subscriptions by apiBasePath
2. Create `internal/controller/apisubscription.go`:
   - `NewApiSubscriptionController(stores) ApiSubscriptionController`
   - `Get(ctx, applicationId, name)` — resolve namespace, store.Get, resolve cross-resources, map
   - `GetAll(ctx, applicationId, params)` — resolve namespace, label filter, fetchAll, batch-resolve, paginate, map
   - `GetStatus(ctx, applicationId, name)` — store.Get, map status
3. Implement batch-resolve helper for cross-resource lookups:
   - Collect unique Zone/Application/Approval refs from subscription list
   - Resolve all in one pass
   - Build lookup maps for O(1) access during mapping

**Deliverables:**
- Both controllers fully implemented
- Batch-resolve pattern for subscription cross-resource lookups

### 🚧 Gate 7: Quality Check + Test Coverage
- [ ] Unit tests with mock stores for all controller methods
- [ ] Test applicationId parsing → correct namespace + label filter
- [ ] Test batch-resolve collects unique refs and handles missing resources gracefully
- [ ] Test GetSubscriptions filters by apiBasePath correctly
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for controller package)

---

## Step 8: Server Layer — HTTP Handlers + Security + Routing

**Goal:** Implement Fiber HTTP handlers, security configuration, and route registration.

**Tasks:**
1. Create `internal/server/server.go`:
   - Controller interfaces (as designed in §4.2.1)
   - `Server` struct with Config, Log, controller fields
   - `securityOpts()` — security templates with `applicationId` key
   - `RegisterRoutes(router)` — register all routes with middleware
2. Create `internal/server/apiexposure_server.go`:
   - `GetAllApiExposures`, `GetApiExposure`, `GetApiExposureStatus`, `GetApiExposureSubscriptions`
   - Parse path/query params from Fiber context
   - Set `X-Total-Count` and `X-Result-Count` headers on list responses
3. Create `internal/server/apisubscription_server.go`:
   - `GetAllApiSubscriptions`, `GetApiSubscription`, `GetApiSubscriptionStatus`
4. Create `internal/server/deprecated.go`:
   - `registerDeprecatedRoutes()` — return 410 Gone for all write endpoints
5. Verify OpenAPI request validation middleware works with the generated embedded spec

**Deliverables:**
- All 7 active endpoints wired
- All 7 deprecated endpoints returning 410
- Security middleware configured with correct templates

### 🚧 Gate 8: Quality Check
- [ ] `go build ./...` passes
- [ ] All routes registered (verify with route table dump or test)
- [ ] Deprecated endpoints return 410 with Problem Details body
- [ ] Run **quality-check** skill

---

## Step 9: Entry Point + Integration Wiring

**Goal:** Wire everything together in `cmd/main.go` and verify the server starts.

**Tasks:**
1. Implement `cmd/main.go`:
   - Load config
   - Initialize logger
   - Create stores
   - Create controllers with stores
   - Create server with controllers
   - Register routes
   - Start Fiber app with graceful shutdown
2. Create `Dockerfile` (adapt from rover-server)
3. Verify server starts locally:
   - `go run ./cmd/main.go` (with mock/dev k8s config)
   - Hit health/probes endpoint
4. Verify OpenAPI validation rejects malformed requests

**Deliverables:**
- Working `cmd/main.go`
- Server starts and responds to health checks
- `Dockerfile` for container builds

### 🚧 Gate 9: Integration Verification
- [ ] Server starts without panics
- [ ] Health/probes endpoint returns 200
- [ ] `GET /applications/test--team--app/apiexposures` returns valid JSON (even if empty)
- [ ] `POST /applications/test--team--app/apiexposures` returns 410 Gone
- [ ] Invalid path returns 404
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 70% overall for spy-server)

---

## Step 10: Documentation & Cleanup

**Goal:** Final documentation, cleanup, and review.

**Tasks:**
1. Add SPDX license headers to all new files
2. Update `spy-server/README.md` with:
   - Overview
   - Build instructions
   - Configuration reference
   - API endpoint summary
3. Run `pre-commit run --all-files` and fix any issues
4. Run full linter suite: `golangci-lint run ./...`
5. Verify REUSE compliance
6. Review all `TODO` comments and create follow-up issues for:
   - CRD extension for `variant` field (Issue 2)
   - Obfuscated access type support
   - ConsumeRouteStore for failover URLs (if needed)
   - Performance testing with large application datasets

**Deliverables:**
- Clean, documented codebase
- All license headers in place
- Follow-up issues documented

### 🚧 Gate 10: Final Review
- [ ] `pre-commit run --all-files` passes
- [ ] `golangci-lint run ./...` passes
- [ ] REUSE compliance verified
- [ ] No `TODO` items without corresponding follow-up issues
- [ ] Run **quality-check** skill (final pass)
- [ ] Run **test-coverage** skill (final: 70% overall minimum)

---

## Summary

| Step | Description | Depends On | Gate |
|------|-------------|------------|------|
| 0 | Project scaffolding + code gen | — | Build + idempotent generate |
| 1 | Store layer | Step 0 | Build + all stores resolve |
| 2 | Configuration + logging | Step 0 | Build |
| 3 | Mapper utilities + status mapper | Step 0 | Tests + 80% coverage |
| 4 | ApiExposure out-mapper | Step 3 | Tests + 80% coverage |
| 5 | ApiSubscription out-mapper | Step 3 | Tests + 80% coverage |
| 6 | Pagination adapter | Step 0 | Tests + 90% coverage |
| 7 | Controllers | Steps 1, 4, 5, 6 | Tests + 80% coverage |
| 8 | Server handlers + security + routing | Steps 2, 7 | Build + route verification |
| 9 | Entry point + integration | Step 8 | Server starts + endpoints respond |
| 10 | Documentation + cleanup | Step 9 | Linting + REUSE + final coverage |

**Parallelism opportunities:** Steps 1, 2, 3, and 6 can all be developed in parallel after Step 0. Steps 4 and 5 can be developed in parallel after Step 3.
