<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Application Resource — Implementation Plan

> **Depends on:** [design.md](./design.md), [000_initial](../000_initial/plan.md) (completed)  
> **Estimated steps:** 5  
> **Status:** Not started

---

## Step 1: Application Out-Mapper

**Goal:** Implement the CRD → API response mapper for Application.

**Tasks:**
1. Create `internal/mapper/application/out.go`:
   - `MapResponse(in *applicationv1.Application) api.ApplicationResponse`
   - Map `id` via `mapper.MakeResourceName(in)` → `<group>--<team>--<appName>`
   - Map `name` from `obj.GetName()`
   - Map `team` from namespace (hub/group) + CRD spec (team name, email) + empty category
   - Map `zone` from `obj.Spec.Zone.Name`
   - Map `status` via `status.MapStatus(conditions, generation)`
   - Map `security` (IpRestrictions) — nil-safe
   - Leave `icto`, `apid`, `psiid` empty (deferred)
2. Create helper `mapSecurity(in, out)` for IpRestrictions mapping

**Deliverables:**
- Complete Application out-mapper
- All fields mapped per design §4.3.2

### 🚧 Gate 1: Quality Check + Test Coverage
- [ ] Unit tests with sample Application CRD objects → expected API responses
- [ ] Test nil security doesn't cause panics
- [ ] Test id construction from namespace + name
- [ ] Test team mapping (hub from namespace, name from spec, email from spec, empty category)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for application mapper)

---

## Step 2: Application Controller

**Goal:** Implement the controller for Application read operations.

**Tasks:**
1. Create `internal/controller/application.go`:
   - `NewApplicationController(stores) server.ApplicationController`
   - Compile-time interface check: `var _ server.ApplicationController = (*applicationController)(nil)`
2. Implement `Get(ctx, applicationId)`:
   - `mapper.ParseApplicationId(ctx, applicationId)` → namespace + appName
   - `stores.ApplicationStore.Get(ctx, namespace, appName)`
   - `applicationmapper.MapResponse(app)`
3. Implement `GetAll(ctx, params)`:
   - `store.NewListOpts()`
   - `store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)`
   - `pagination.FetchAll(ctx, stores.ApplicationStore, listOpts)`
   - Map all items via `applicationmapper.MapResponse`
   - `pagination.Paginate(mapped, params.Offset, params.Limit, "/applications")`
   - Return `ApplicationListResponse`
   - **Note:** `icto`, `apid`, `psiid` params are accepted but ignored for now
4. Implement `GetStatus(ctx, applicationId)`:
   - Parse applicationId → namespace + appName
   - `stores.ApplicationStore.Get(ctx, namespace, appName)`
   - `statusmapper.MapResponse(app)`

**Deliverables:**
- Complete `ApplicationController` implementation
- All 3 methods (Get, GetAll, GetStatus)

### 🚧 Gate 2: Quality Check + Test Coverage
- [ ] Unit tests with mock stores for all 3 controller methods
- [ ] Test `GetAll` uses `PrefixFromContext` correctly
- [ ] Test `Get` resolves applicationId → namespace + name correctly
- [ ] Test `GetStatus` returns correct status mapping
- [ ] Test error handling (store errors, invalid applicationId)
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 80% for controller package)

---

## Step 3: Server Layer — HTTP Handlers + Route Registration

**Goal:** Add Application HTTP handlers, update the Server struct, and register routes.

**Tasks:**
1. Update `internal/server/server.go`:
   - Add `ApplicationController` interface
   - Add `Applications ApplicationController` field to `Server` struct
2. Create `internal/server/application_server.go`:
   - `GetAllApplications(c *fiber.Ctx) error` — parse query params, call controller, set headers, return JSON
   - `GetApplication(c *fiber.Ctx) error` — parse path param, call controller, return JSON
   - `GetApplicationStatus(c *fiber.Ctx) error` — parse path param, call controller, return JSON
3. Update `RegisterRoutes` in `server.go`:
   - Add Application read routes:
     ```
     router.Get("/applications", checkAccess, s.GetAllApplications)
     router.Get("/applications/:applicationId", checkAccess, s.GetApplication)
     router.Get("/applications/:applicationId/status", checkAccess, s.GetApplicationStatus)
     ```
4. Update `registerDeprecatedRoutes` in `deprecated.go`:
   - Add Application write routes:
     ```
     router.Post("/applications", checkAccess, deprecatedHandler)
     router.Put("/applications/:applicationId", checkAccess, deprecatedHandler)
     router.Delete("/applications/:applicationId", checkAccess, deprecatedHandler)
     ```

**Deliverables:**
- All 3 active Application endpoints wired
- All 3 deprecated Application endpoints returning 410
- Server struct updated with new controller

### 🚧 Gate 3: Quality Check
- [ ] `go build ./...` passes
- [ ] All new routes registered (verify route table)
- [ ] Deprecated Application endpoints return 410 with Problem Details body
- [ ] Run **quality-check** skill

---

## Step 4: Entry Point Wiring

**Goal:** Wire the Application controller into `cmd/main.go`.

**Tasks:**
1. Update `cmd/main.go`:
   - Import `controller` package (already imported)
   - Create Application controller: `controller.NewApplicationController(stores)`
   - Pass to Server struct: `Applications: controller.NewApplicationController(stores)`
2. Verify server starts with new routes
3. Verify `GET /applications` returns valid JSON (even if empty)
4. Verify `GET /applications/{applicationId}` with a valid format returns 404 (no data) not 500
5. Verify `POST /applications` returns 410 Gone

**Deliverables:**
- Working `cmd/main.go` with Application controller wired
- Server starts and responds correctly

### 🚧 Gate 4: Integration Verification
- [ ] Server starts without panics
- [ ] `GET /applications` returns valid JSON response (empty items list is OK)
- [ ] `GET /applications/test--team--app` returns 404 (not 500)
- [ ] `GET /applications/test--team--app/status` returns 404 (not 500)
- [ ] `POST /applications` returns 410 Gone
- [ ] `PUT /applications/test--team--app` returns 410 Gone
- [ ] `DELETE /applications/test--team--app` returns 410 Gone
- [ ] Run **quality-check** skill
- [ ] Run **test-coverage** skill (target: 70% overall for spy-server)

---

## Step 5: Documentation & Cleanup

**Goal:** Final documentation, license headers, and cleanup.

**Tasks:**
1. Add SPDX license headers to all new files
2. Run `pre-commit run --all-files` and fix any issues
3. Run `golangci-lint run ./...` and fix any issues
4. Verify REUSE compliance
5. Document deferred items as follow-up TODOs:
   - [ ] CRD extension for `icto`, `apid`, `psiid` fields + filtering
   - [ ] Source for `Team.category` field
   - [ ] Obfuscated access type support

**Deliverables:**
- Clean, documented new code
- All license headers in place
- Deferred items documented

### 🚧 Gate 5: Final Review
- [ ] `pre-commit run --all-files` passes
- [ ] `golangci-lint run ./...` passes
- [ ] REUSE compliance verified
- [ ] Run **quality-check** skill (final pass)
- [ ] Run **test-coverage** skill (final: 70% overall minimum)

---

## Summary

| Step | Description | Depends On | Gate |
|------|-------------|------------|------|
| 1 | Application out-mapper | 000_initial completed | Tests + 80% coverage |
| 2 | Application controller | Step 1 | Tests + 80% coverage |
| 3 | Server handlers + routes | Step 2 | Build + route verification |
| 4 | Entry point wiring | Step 3 | Server starts + endpoints respond |
| 5 | Documentation + cleanup | Step 4 | Linting + REUSE + final coverage |

**Parallelism:** Steps 1 and 2 are sequential (controller depends on mapper). Steps 3–5 are sequential. No parallelism opportunities in this iteration since the feature is a single vertical slice.

**Estimated effort:** Small — ~5 files changed/created, no new stores, no new dependencies, following established patterns exactly.
