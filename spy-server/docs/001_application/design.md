<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Application Resource — Design Document

> **Version:** 1.0  
> **Date:** 2025-03-30  
> **Status:** Accepted  
> **Component:** `spy-server/`  
> **Depends on:** [000_initial design](../000_initial/design.md)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Scope](#2-scope)
3. [Architecture](#3-architecture)
4. [Layer-by-Layer Design](#4-layer-by-layer-design)
   - [4.1 Server Layer (HTTP Handlers)](#41-server-layer-http-handlers)
   - [4.2 Controller Layer](#42-controller-layer)
   - [4.3 Mapper Layer](#43-mapper-layer)
   - [4.4 Store Layer](#44-store-layer)
5. [Security Model](#5-security-model)
6. [Pagination](#6-pagination)
7. [Deprecated Endpoints](#7-deprecated-endpoints)
8. [Potential Issues & Risks](#8-potential-issues--risks)
9. [Decision Log](#9-decision-log)

---

## 1. Overview

This document describes the design for adding **Application** resource endpoints to the spy-server. The Application resource represents a TARDIS application and is the **top-level** entity under which ApiExposures and ApiSubscriptions are scoped.

The Application endpoints follow the same read-only pattern established in [000_initial](../000_initial/design.md) but differ in one key way: the list endpoint (`GET /applications`) is a **global** resource listing — it has no `applicationId` path parameter and returns all applications visible to the caller.

### Key Facts

| Aspect | Value |
|--------|-------|
| OpenAPI spec | `api/application-openapi.yaml` |
| API base path | `/application/v2` (merged into `/stargate/v2`) |
| Resource scope | `/applications` (top-level) and `/applications/{applicationId}` |
| Active endpoints | 3 GET endpoints |
| Deprecated endpoints | 3 POST/PUT/DELETE endpoints |
| New controller interface | `ApplicationController` |
| CRD source | `application.cp.ei.telekom.de/v1` → `Application` |

---

## 2. Scope

### In Scope

- `GET /applications` — List all applications (with pagination)
- `GET /applications/{applicationId}` — Get a single application
- `GET /applications/{applicationId}/status` — Get application status
- `POST /applications` — Deprecated (410 Gone)
- `PUT /applications/{applicationId}` — Deprecated (410 Gone)
- `DELETE /applications/{applicationId}` — Deprecated (410 Gone)
- CRD → API response mapping (Application → ApplicationResponse)
- Security integration via `checkAccess` + `PrefixFromContext`

### Out of Scope (Deferred)

- **`icto`, `apid`, `psiid` fields:** These are defined in the OpenAPI spec but not yet present in the Application CRD. They will be left empty in responses and filter parameters will be accepted but ignored until the CRD is extended.
- **`Team.category` field:** Not available in the CRD; will default to empty string.
- **Obfuscated access type:** Field masking for `obfuscated` scope is deferred (same as 000_initial).

---

## 3. Architecture

### 3.1 Data Flow

The Application resource follows the identical layered architecture from 000_initial:

```
HTTP Request
    │
    ▼
┌─────────────────────┐
│   Fiber Middleware   │  ← JWT, BusinessContext, CheckAccess, OpenAPI validation
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│   Server Layer      │  ← Parse path/query params, delegate to controller
│  (application_      │
│   server.go)        │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Controller Layer   │  ← Orchestrate: parse applicationId, query store, invoke mapper
│  (application.go)   │
└─────────┬───────────┘
          │
          ▼
┌──────────┴──────────┐
│                     │
▼                     ▼
┌───────────┐   ┌───────────┐
│  Mapper   │   │   Store   │  ← ApplicationStore (already exists)
│ (app/out) │   │           │
└───────────┘   └───────────┘
```

### 3.2 Key Differences from ApiExposure/ApiSubscription

| Aspect | ApiExposure / ApiSubscription | Application |
|--------|-------------------------------|-------------|
| List endpoint path | `/applications/{applicationId}/apiexposures` | `/applications` |
| Has `applicationId` path param in list | Yes | **No** |
| Scoping mechanism (list) | Namespace prefix + application label | **Namespace prefix only** (via `PrefixFromContext`) |
| Security on list | Namespaced request (checkAccess with applicationId) | **Global request** (checkAccess without path param) |
| Cross-resource lookups | Zone, Approval, Application stores | **Zone store only** (for zone name) |
| applicationId construction | Provided by caller | **Constructed from CRD** (`MakeResourceName`) |

---

## 4. Layer-by-Layer Design

### 4.1 Server Layer (HTTP Handlers)

#### 4.1.1 Controller Interface

Added to `internal/server/server.go`:

```go
// ApplicationController defines the read-only operations for Application resources.
type ApplicationController interface {
    Get(ctx context.Context, applicationId string) (api.ApplicationResponse, error)
    GetAll(ctx context.Context, params api.GetAllApplicationsParams) (*api.ApplicationListResponse, error)
    GetStatus(ctx context.Context, applicationId string) (api.ResourceStatusResponse, error)
}
```

#### 4.1.2 Server Struct Extension

```go
type Server struct {
    Config           *config.ServerConfig
    Log              logr.Logger
    ApiExposures     ApiExposureController
    ApiSubscriptions ApiSubscriptionController
    Applications     ApplicationController       // ← NEW
}
```

#### 4.1.3 Handler Pattern

The handlers follow the exact same pattern as ApiExposure handlers:

```go
// GetAllApplications handles GET /applications
func (s *Server) GetAllApplications(c *fiber.Ctx) error {
    params := api.GetAllApplicationsParams{}
    if err := c.QueryParser(&params); err != nil {
        return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
    }

    resp, err := s.Applications.GetAll(c.UserContext(), params)
    if err != nil {
        return cserver.ReturnWithProblem(c, nil, err)
    }

    c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
    c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
    return c.JSON(resp)
}

// GetApplication handles GET /applications/:applicationId
func (s *Server) GetApplication(c *fiber.Ctx) error {
    applicationId := c.Params("applicationId")

    resp, err := s.Applications.Get(c.UserContext(), applicationId)
    if err != nil {
        return cserver.ReturnWithProblem(c, nil, err)
    }
    return c.JSON(resp)
}

// GetApplicationStatus handles GET /applications/:applicationId/status
func (s *Server) GetApplicationStatus(c *fiber.Ctx) error {
    applicationId := c.Params("applicationId")

    resp, err := s.Applications.GetStatus(c.UserContext(), applicationId)
    if err != nil {
        return cserver.ReturnWithProblem(c, nil, err)
    }
    return c.JSON(resp)
}
```

#### 4.1.4 Route Registration

Added to `RegisterRoutes`:

```go
// Application routes (read-only)
s.Log.Info("Registering application routes")
router.Get("/applications", checkAccess, s.GetAllApplications)
router.Get("/applications/:applicationId", checkAccess, s.GetApplication)
router.Get("/applications/:applicationId/status", checkAccess, s.GetApplicationStatus)
```

> **Note:** The `checkAccess` middleware is used on all routes including `GET /applications`. When `applicationId` is empty (no path param on the list endpoint), the middleware detects empty params via `areParamsEmpty()` and calls `CheckGlobalRequest()`, which grants access based on client type + access type. The middleware still computes and stores the prefix in context, which is used by `PrefixFromContext()` in the controller.

### 4.2 Controller Layer

#### 4.2.1 GetAll — Global List with Prefix Scoping

The `GET /applications` endpoint has no `applicationId` path parameter. Security scoping works via:

1. `checkAccess` middleware detects empty params → calls `CheckGlobalRequest()`
2. The middleware computes a prefix from the `ExpectedTemplate` and stores it in context
3. The controller retrieves the prefix via `security.PrefixFromContext(ctx)` and uses `store.EnforcePrefix()` to scope the store query

This is **identical** to how rover-server's `GET /rovers` works.

```go
func (c *applicationController) GetAll(ctx context.Context, params api.GetAllApplicationsParams) (*api.ApplicationListResponse, error) {
    listOpts := store.NewListOpts()
    store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

    items, err := pagination.FetchAll(ctx, c.stores.ApplicationStore, listOpts)
    if err != nil {
        return nil, err
    }

    mapped := make([]api.ApplicationResponse, len(items))
    for i, item := range items {
        mapped[i] = applicationmapper.MapResponse(item)
    }

    basePath := "/applications"
    result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

    return &api.ApplicationListResponse{
        Items:           result.Items,
        Paging:          result.Paging,
        UnderscoreLinks: result.Links,
    }, nil
}
```

#### 4.2.2 Get — Single Application

The `GET /applications/:applicationId` handler uses the existing `ParseApplicationId` to resolve namespace + name, then fetches from the ApplicationStore:

```go
func (c *applicationController) Get(ctx context.Context, applicationId string) (api.ApplicationResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    if err != nil {
        return api.ApplicationResponse{}, err
    }

    app, err := c.stores.ApplicationStore.Get(ctx, appInfo.Namespace, appInfo.AppName)
    if err != nil {
        return api.ApplicationResponse{}, err
    }

    return applicationmapper.MapResponse(app), nil
}
```

> **Note:** Unlike ApiExposure/ApiSubscription, the Application name in the store is just `<appName>` (not `<appName>--<subresource>`), so no full-name construction is needed. The VerifyApplicationLabel check is also unnecessary — the application IS the application.

#### 4.2.3 GetStatus

Reuses the existing status mapper:

```go
func (c *applicationController) GetStatus(ctx context.Context, applicationId string) (api.ResourceStatusResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    if err != nil {
        return api.ResourceStatusResponse{}, err
    }

    app, err := c.stores.ApplicationStore.Get(ctx, appInfo.Namespace, appInfo.AppName)
    if err != nil {
        return api.ResourceStatusResponse{}, err
    }

    return statusmapper.MapResponse(app), nil
}
```

### 4.3 Mapper Layer

#### 4.3.1 Application Out-Mapper

A new mapper at `internal/mapper/application/out.go`:

```go
func MapResponse(in *applicationv1.Application) api.ApplicationResponse {
    nsInfo := mapper.ParseNamespace(in.GetNamespace())

    resp := api.ApplicationResponse{
        Id:   mapper.MakeResourceName(in),   // <group>--<team>--<appName>
        Name: in.GetName(),
        Team: api.Team{
            Hub:      nsInfo.Group,
            Name:     in.Spec.Team,
            Email:    openapi_types.Email(in.Spec.TeamEmail),
            Category: "",                     // Deferred — not in CRD
        },
        Zone:   in.Spec.Zone.Name,
        Status: status.MapStatus(in.GetConditions(), in.GetGeneration()),
        // icto, apid, psiid: left empty (deferred)
    }

    mapSecurity(in, &resp)
    return resp
}
```

#### 4.3.2 Field Mapping Summary

| API field | CRD source | Notes |
|-----------|-----------|-------|
| `id` | `MakeResourceName(obj)` → `<group>--<team>--<name>` | Derived from namespace + name |
| `name` | `obj.GetName()` | Direct |
| `team.hub` | `ParseNamespace(obj.Namespace).Group` | Derived from namespace |
| `team.name` | `obj.Spec.Team` | Direct |
| `team.email` | `obj.Spec.TeamEmail` | Direct |
| `team.category` | `""` | **Deferred** — not in CRD |
| `zone` | `obj.Spec.Zone.Name` | ObjectRef → name only |
| `icto` | `""` | **Deferred** — not in CRD |
| `apid` | `""` | **Deferred** — not in CRD |
| `psiid` | `""` | **Deferred** — not in CRD |
| `status` | `status.MapStatus(conditions, generation)` | Reuses existing mapper |
| `security` | `obj.Spec.Security` | Map IpRestrictions |

#### 4.3.3 Security Mapping

The CRD `Security` has `IpRestrictions` with `Allow` and `Deny` slices. The API `Security` only has `IpRestrictions` with `Allow`:

```go
func mapSecurity(in *applicationv1.Application, out *api.ApplicationResponse) {
    if in.Spec.Security == nil || in.Spec.Security.IpRestrictions == nil {
        return
    }

    out.Security = api.Security{
        IpRestrictions: api.IpRestrictions{
            Allow: in.Spec.Security.IpRestrictions.Allow,
        },
    }
}
```

### 4.4 Store Layer

No changes needed — `ApplicationStore` already exists in `pkg/store/stores.go`:

```go
type Stores struct {
    APIExposureStore     store.ObjectStore[*apiv1.ApiExposure]
    APISubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
    ApplicationStore     store.ObjectStore[*applicationv1.Application]  // ← already exists
    ZoneStore            store.ObjectStore[*adminv1.Zone]
    ApprovalStore        store.ObjectStore[*approvalv1.Approval]
}
```

---

## 5. Security Model

### 5.1 Reusing Existing Security Configuration

The existing `securityTemplates` in `server.go` use `applicationId` as the path param key. This works for all Application endpoints:

- **`GET /applications/{applicationId}`** and **`GET /applications/{applicationId}/status`**: The `applicationId` path param is present → namespaced request → `CheckNamespacedRequest()` validates prefix match.
- **`GET /applications`**: No path params → `areParamsEmpty()` returns true → `CheckGlobalRequest()` grants based on client type + access type → prefix is computed from `ExpectedTemplate` and stored in context.

### 5.2 Prefix Resolution for List

The prefix stored by `checkAccess` matches the store key format:

| Client Type | ExpectedTemplate | Computed Prefix | Store Key Format |
|-------------|-----------------|-----------------|------------------|
| Team | `{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}--` | `dev--eni--hyperion/` | `dev--eni--hyperion/<appName>` |
| Group | `{{ .B.Environment }}--{{ .B.Group }}--` | `dev--eni--` | `dev--eni--*/<appName>` |
| Admin | `{{ .B.Environment }}--` | `dev--` | `dev--*/<appName>` |

> **Note:** For team-level clients, the `toDatastorePrefix` function (in `check_access.go`) converts the trailing `--` to `/` to match the store's `<namespace>/<name>` key format.

The controller then uses `store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)` to scope the store query — identical to rover-server's pattern.

---

## 6. Pagination

Same in-memory pagination adapter as 000_initial:

1. `pagination.FetchAll()` drains the store's cursor-based pagination
2. `pagination.Paginate()` applies offset/limit and builds HATEOAS links
3. Server handler sets `X-Total-Count` and `X-Result-Count` headers

The `basePath` for link construction is `/applications`.

---

## 7. Deprecated Endpoints

Three deprecated endpoints return HTTP 410 Gone:

```go
// Application write endpoints
router.Post("/applications", checkAccess, deprecatedHandler)
router.Put("/applications/:applicationId", checkAccess, deprecatedHandler)
router.Delete("/applications/:applicationId", checkAccess, deprecatedHandler)
```

These are added to the existing `registerDeprecatedRoutes()` method.

---

## 8. Potential Issues & Risks

### 🟡 Important

#### Issue 1: `icto`, `apid`, `psiid` Filter Parameters Accepted but Ignored
**Problem:** The OpenAPI spec defines `icto`, `apid`, `psiid` as query filters on `GET /applications`. The `GetAllApplicationsParams` struct includes these fields, but we have no CRD data to filter against.  
**Impact:** Callers providing these filters will get unfiltered results (silent no-op).  
**Mitigation:** Accept the parameters silently. When the CRD is extended to include these fields, add store-level filtering via annotations/labels or CRD spec fields.

#### Issue 2: `Team.category` Always Empty
**Problem:** The API requires `category` in the Team object, but the CRD has no source for it.  
**Impact:** The `category` field will be empty in all responses.  
**Mitigation:** Accept for now. When the source is identified (separate CRD, annotation, or field extension), populate it.

#### Issue 3: Application List May Be Large
**Problem:** Unlike ApiExposure/ApiSubscription (scoped per application, typically <100 items), the Application list can span all applications across all teams in the environment.  
**Impact:** The in-memory `FetchAll` could load many thousands of applications for admin-level clients.  
**Mitigation:** The in-memory pagination is still appropriate since:
- Team-level clients are scoped to their namespace (bounded)
- Group-level clients are scoped to their group (moderate)
- Admin-level clients may see many applications, but the pagination adapter limits what's returned per page  
- Consider adding a hard cap (e.g., 10,000 items) in `FetchAll` as a safety valve (already documented in 000_initial).

### 🟢 Minor

#### Issue 4: Route Ordering with Existing `/applications/:applicationId/...` Routes
**Problem:** The new `GET /applications` route must be registered carefully relative to existing `/applications/:applicationId/apiexposures` routes.  
**Impact:** Fiber matches routes top-down. If `/applications` is registered after `/applications/:applicationId/apiexposures`, no conflict since they're different HTTP paths. But the deprecated `POST /applications` must not conflict with existing routes.  
**Mitigation:** Register Application routes in the correct order within `RegisterRoutes`. Fiber's path matching is deterministic — `/applications` (exact) vs `/applications/:applicationId/...` (parameterized) do not conflict.

---

## 9. Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Use `checkAccess` + `PrefixFromContext` for list endpoint | Same pattern as rover-server's `GET /rovers`; the middleware already handles global requests (empty params) and computes the correct prefix |
| D2 | Leave `icto`, `apid`, `psiid` empty in responses | Not in CRD; deferred to future CRD extension |
| D3 | Leave `Team.category` empty | Source not yet identified; deferred |
| D4 | No new stores needed | `ApplicationStore` already exists in `Stores` container |
| D5 | No cross-resource lookups needed for Application | Zone name is directly on the CRD spec (`Spec.Zone.Name`); no store lookup required |
| D6 | No `VerifyApplicationLabel` check for Application | The Application IS the application — no label scoping needed unlike sub-resources |
| D7 | Reuse existing `securityTemplates` without modification | Templates use `applicationId` path param key; `checkAccess` handles empty params via `CheckGlobalRequest` |
| D8 | Add deprecated Application write routes to existing `registerDeprecatedRoutes` | Consistent with 000_initial pattern |
