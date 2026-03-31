<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# spy-server Design Document

> **Version:** 1.0  
> **Date:** 2025-03-30  
> **Status:** Accepted  
> **Component:** `spy-server/`  
> **Reference Implementation:** `rover-server/`

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Directory Structure](#3-directory-structure)
4. [Layer-by-Layer Design](#4-layer-by-layer-design)
   - [4.1 Code Generation (OpenAPI)](#41-code-generation-openapi)
   - [4.2 Server Layer (HTTP Handlers)](#42-server-layer-http-handlers)
   - [4.3 Controller Layer](#43-controller-layer)
   - [4.4 Service + Mapper Layer](#44-service--mapper-layer)
   - [4.5 Store Layer](#45-store-layer)
5. [Security Model](#5-security-model)
6. [Pagination Strategy](#6-pagination-strategy)
7. [Deprecated Endpoints](#7-deprecated-endpoints)
8. [Potential Issues & Risks](#8-potential-issues--risks)
9. [Decision Log](#9-decision-log)

---

## 1. Overview

The **spy-server** is a **read-only** REST API server that exposes `ApiExposure` and `ApiSubscription` resources scoped under applications. It follows the same layered architecture as `rover-server`:

```
Generated HTTP types  →  Server (Fiber handlers)  →  Controller  →  Mapper  →  Store (common-server)
```

The API is consumed by teams wanting to inspect their TARDIS API configurations without write access — all mutations are performed through the Rover API.

### Key Facts

| Aspect | Value |
|--------|-------|
| OpenAPI version | 3.0.3 |
| API base path | `/stargate/v2` |
| Resource scope | `/applications/{applicationId}/...` |
| Pagination model | **Offset/limit** with HATEOAS links |
| Active endpoints | 7 GET endpoints |
| Deprecated endpoints | 7 POST/PUT/DELETE endpoints |
| Default security scope | `tardis:user:read` |

---

## 2. Architecture

### 2.1 High-Level Data Flow

```
HTTP Request
    │
    ▼
┌─────────────────────┐
│   Fiber Middleware   │  ← JWT validation, BusinessContext, CheckAccess, OpenAPI validation
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│   Server Layer      │  ← Parse path/query params, delegate to controller, return JSON/Problem
│   (HTTP Handlers)   │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│   Controller Layer   │  ← Orchestrate business logic, call stores, invoke mappers
└─────────┬───────────┘
          │
          ▼
┌──────────┴──────────┐
│                     │
▼                     ▼
┌───────────┐   ┌───────────┐
│  Mapper   │   │   Store   │  ← ObjectStore[T] from common-server (in-memory + k8s watch)
│  (out/)   │   │           │
└───────────┘   └───────────┘
```

### 2.2 Key Differences from rover-server

| Aspect | rover-server | spy-server |
|--------|-------------|-----------------|
| Resource scope | Flat `/:resourceId` | Nested `/applications/{applicationId}/...` |
| Pagination | Cursor-based | Offset/limit with HATEOAS |
| Write operations | Full CRUD | Read-only (writes deprecated) |
| In-mappers | Yes (`in/`) | Not needed |
| Path param key | `resourceId` | `applicationId` + `apiExposureName`/`apiSubscriptionName` |
| Cross-resource lookups | Minimal | Required (Zone, Approval, Application for subscriptions) |

---

## 3. Directory Structure

```
spy-server/
├── api/                          # ← renamed from top-level; holds OpenAPI spec
│   └── openapi.yaml
├── cmd/
│   └── main.go                   # Entry point
├── internal/
│   ├── api/
│   │   └── server.gen.go         # Generated types + embedded spec (oapi-codegen)
│   ├── config/
│   │   └── config.go             # Server configuration (Viper-based)
│   ├── server/
│   │   ├── server.go             # Controller interfaces, security config, route registration
│   │   ├── apiexposure_server.go # HTTP handlers for ApiExposure endpoints
│   │   ├── apisubscription_server.go # HTTP handlers for ApiSubscription endpoints
│   │   └── deprecated.go         # Handlers for deprecated write endpoints
│   ├── controller/
│   │   ├── apiexposure.go        # ApiExposure controller implementation
│   │   ├── apisubscription.go    # ApiSubscription controller implementation
│   │   └── util.go               # Shared helpers (pagination adapter, application lookup)
│   └── mapper/
│       ├── apiexposure/
│       │   └── out/
│       │       └── apiexposure.go  # CRD → ApiExposureResponse
│       ├── apisubscription/
│       │   └── out/
│       │       └── apisubscription.go # CRD → ApiSubscriptionResponse
│       ├── status/
│       │   └── response.go        # Condition → StatusResponse mapping
│       └── util.go                # ParseApplicationId, shared helpers
├── pkg/
│   ├── log/
│   │   └── log.go                # Logger setup
│   └── store/
│       └── stores.go             # Store dependency container
├── tools/
│   ├── generate.go               # //go:generate directive
│   └── server.yaml               # oapi-codegen config
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

---

## 4. Layer-by-Layer Design

### 4.1 Code Generation (OpenAPI)

**Approach:** Use `oapi-codegen` with the same pattern as rover-server — generate **types + embedded spec only** (no server/strict-server generation). Hand-written Fiber handlers give us full control.

**Config (`tools/server.yaml`):**

```yaml
package: api
output: internal/api/server.gen.go
generate:
  models: true
  embedded-spec: true
  fiber-server: false
  strict-server: false
```

**`tools/generate.go`:**

```go
package tools

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config server.yaml ../api/openapi.yaml
```

**Generated artifacts:**
- All request/response model types (e.g., `ApiExposureResponse`, `ApiSubscriptionResponse`, `ApiExposureListResponse`, `Paging`, `Links`, etc.)
- Embedded OpenAPI spec for runtime validation

### 4.2 Server Layer (HTTP Handlers)

The server layer is a thin translation layer: parse Fiber context → call controller → return JSON or RFC 7807 problem.

#### 4.2.1 Controller Interfaces

Defined in `internal/server/server.go`:

```go
type ApiExposureController interface {
    Get(ctx context.Context, applicationId, apiExposureName string) (api.ApiExposureResponse, error)
    GetAll(ctx context.Context, applicationId string, params api.GetAllApiExposuresParams) (*api.ApiExposureListResponse, error)
    GetStatus(ctx context.Context, applicationId, apiExposureName string) (api.ResourceStatusResponse, error)
    GetSubscriptions(ctx context.Context, applicationId, apiExposureName string) ([]api.ApiSubscriptionResponse, error)
}

type ApiSubscriptionController interface {
    Get(ctx context.Context, applicationId, apiSubscriptionName string) (api.ApiSubscriptionResponse, error)
    GetAll(ctx context.Context, applicationId string, params api.GetAllApiSubscriptionsParams) (*api.ApiSubscriptionListResponse, error)
    GetStatus(ctx context.Context, applicationId, apiSubscriptionName string) (api.ResourceStatusResponse, error)
}
```

#### 4.2.2 Handler Pattern

Each handler follows the rover-server pattern:

```go
func (s *Server) GetApiExposure(c *fiber.Ctx) error {
    applicationId := c.Params("applicationId")
    apiExposureName := c.Params("apiExposureName")

    resp, err := s.ApiExposures.Get(c.UserContext(), applicationId, apiExposureName)
    if err != nil {
        return server.ReturnWithProblem(c, err)
    }
    return c.JSON(resp)
}
```

#### 4.2.3 Route Registration

```go
func (s *Server) RegisterRoutes(router fiber.Router) {
    checkAccess := security.ConfigureSecurity(router, s.securityOpts())

    // OpenAPI validation middleware
    router.Use(openapiValidator)

    // ApiExposure routes (read-only)
    router.Get("/applications/:applicationId/apiexposures", checkAccess, s.GetAllApiExposures)
    router.Get("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, s.GetApiExposure)
    router.Get("/applications/:applicationId/apiexposures/:apiExposureName/status", checkAccess, s.GetApiExposureStatus)
    router.Get("/applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions", checkAccess, s.GetApiExposureSubscriptions)

    // ApiSubscription routes (read-only)
    router.Get("/applications/:applicationId/apisubscriptions", checkAccess, s.GetAllApiSubscriptions)
    router.Get("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, s.GetApiSubscription)
    router.Get("/applications/:applicationId/apisubscriptions/:apiSubscriptionName/status", checkAccess, s.GetApiSubscriptionStatus)

    // Deprecated write endpoints (return 410 Gone)
    s.registerDeprecatedRoutes(router, checkAccess)
}
```

### 4.3 Controller Layer

The controller orchestrates business logic: resolve the application, query stores with proper filters, invoke mappers, and handle pagination translation.

#### 4.3.1 ApplicationId Format

The `applicationId` path parameter follows the pattern `<group>--<team>--<applicationName>`. This needs to be parsed to:
- **Namespace:** `<env>--<group>--<team>` (environment comes from BusinessContext)
- **Application name:** `<applicationName>`
- **Label filter:** `cp.ei.telekom.de/application=<applicationName>`

```go
// ParseApplicationId extracts group, team, and appName from the applicationId.
// Format: <group>--<team>--<applicationName>
func ParseApplicationId(applicationId string) (group, team, appName string, err error) {
    parts := strings.SplitN(applicationId, "--", 3)
    if len(parts) != 3 {
        return "", "", "", problems.BadRequest("invalid applicationId format, expected: <group>--<team>--<applicationName>")
    }
    return parts[0], parts[1], parts[2], nil
}
```

#### 4.3.2 Controller Implementation Pattern

```go
type apiExposureController struct {
    stores *store.Stores
}

func (c *apiExposureController) GetAll(ctx context.Context, applicationId string, params api.GetAllApiExposuresParams) (*api.ApiExposureListResponse, error) {
    group, team, appName, err := mapper.ParseApplicationId(applicationId)
    if err != nil {
        return nil, err
    }

    env := security.EnvironmentFromContext(ctx)
    namespace := fmt.Sprintf("%s--%s--%s", env, group, team)

    // Build ListOpts with application label filter
    listOpts := store.NewListOpts()
    listOpts.Prefix = namespace + "/"
    listOpts.Filters = append(listOpts.Filters, store.Filter{
        Path:  "metadata.labels.cp\\.ei\\.telekom\\.de/application",
        Op:    store.OpEqual,
        Value: appName,
    })

    // Fetch all matching items from store (cursor-based)
    allItems, err := fetchAll(ctx, c.stores.APIExposureStore, listOpts)
    if err != nil {
        return nil, err
    }

    // Apply offset/limit pagination
    page := paginate(allItems, params.Offset, params.Limit)

    // Map to response
    items := make([]api.ApiExposureResponse, len(page.Items))
    for i, item := range page.Items {
        items[i] = outmapper.MapApiExposure(item)
    }

    return &api.ApiExposureListResponse{
        Links: page.Links,
        Items: items,
        Paging: page.Paging,
    }, nil
}
```

#### 4.3.3 Cross-Resource Lookups (ApiSubscription)

The `ApiSubscriptionResponse` requires data from multiple sources:

| Response field | Source |
|---------------|--------|
| `name` | ApiSubscription.Name |
| `apiBasePath` | ApiSubscription.Spec.ApiBasePath |
| `security` | ApiSubscription.Spec.Security |
| `application` | Application store lookup via ApiSubscription.Spec.Requestor |
| `team` | Application.Spec.Team |
| `approval` | Approval store lookup via ApiSubscription.Status.Approval |
| `gatewayUrl` | Zone store lookup via ApiSubscription.Spec.Zone → Zone.Status.Links.Url + ApiBasePath |
| `failover` | Additional Zone lookups for failover URLs |

The controller will inject these resolved values into the mapper context:

```go
type SubscriptionMapContext struct {
    Subscription *apiv1.ApiSubscription
    Application  *applicationv1.Application  // may be nil
    Zone         *adminv1.Zone               // may be nil
    Approval     *approvalv1.Approval        // may be nil
}
```

### 4.4 Service + Mapper Layer

Since this is read-only, we only need **out-mappers** (CRD → API response) and **status mappers**.

#### 4.4.1 Out-Mapper: ApiExposure

```go
func MapApiExposure(obj *apiv1.ApiExposure) api.ApiExposureResponse {
    return api.ApiExposureResponse{
        Name:         obj.Name,
        ApiBasePath:  obj.Spec.ApiBasePath,
        Upstreams:    mapUpstreams(obj.Spec.Upstreams),
        Visibility:   mapVisibility(obj.Spec.Visibility),
        Approval:     mapApprovalConfig(obj.Spec.Approval),
        Security:     mapSecurity(obj.Spec.Security),
    }
}
```

#### 4.4.2 Out-Mapper: ApiSubscription

```go
func MapApiSubscription(mctx SubscriptionMapContext) api.ApiSubscriptionResponse {
    sub := mctx.Subscription
    resp := api.ApiSubscriptionResponse{
        Name:        sub.Name,
        ApiBasePath: sub.Spec.ApiBasePath,
        Security:    mapSubscriberSecurity(sub.Spec.Security),
    }

    if mctx.Application != nil {
        resp.Application = &api.ApplicationRef{
            Name: mctx.Application.Name,
            Team: mctx.Application.Spec.Team,
        }
        resp.Team = mctx.Application.Spec.Team
    }

    if mctx.Zone != nil && mctx.Zone.Status.Links != nil {
        resp.GatewayUrl = mctx.Zone.Status.Links.Url + sub.Spec.ApiBasePath
    }

    if mctx.Approval != nil {
        resp.Approval = mapApprovalStatus(mctx.Approval)
    }

    return resp
}
```

#### 4.4.3 Status Mapper

Reuse the rover-server pattern from `internal/mapper/status/`:

```go
func MapStatusResponse(obj metav1.Object, conditions []metav1.Condition) api.ResourceStatusResponse {
    return api.ResourceStatusResponse{
        ProcessingState: mapProcessingState(conditions),
        OverallStatus:   mapOverallStatus(conditions),
        Conditions:      mapConditions(conditions),
    }
}
```

> **Note:** The status mapper can likely be extracted to a shared package or reused from rover-server's mapper/status since the pattern is identical.

### 4.5 Store Layer

#### 4.5.1 Store Container

```go
type Stores struct {
    APIExposureStore    store.ObjectStore[*apiv1.ApiExposure]
    APISubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
    ApplicationStore    store.ObjectStore[*applicationv1.Application]
    ZoneStore           store.ObjectStore[*adminv1.Zone]
    ApprovalStore       store.ObjectStore[*approvalv1.Approval]
}
```

**Why these stores:**

| Store | Purpose |
|-------|---------|
| `APIExposureStore` | Primary resource for ApiExposure endpoints |
| `APISubscriptionStore` | Primary resource for ApiSubscription endpoints |
| `ApplicationStore` | Resolve `applicationId` → Application, provide team/application info in subscription responses |
| `ZoneStore` | Construct `gatewayUrl` = Zone.Status.Links.Url + ApiBasePath |
| `ApprovalStore` | Resolve approval state for subscription responses |

> **Decision:** `ConsumeRouteStore` is **not** included in the initial design. Failover gateway URLs can be resolved from Zone references in the ApiSubscription status without needing ConsumeRoute data directly. If failover URL construction proves to require ConsumeRoute properties, this store can be added later.

#### 4.5.2 Initialization

```go
func NewStores(ctx context.Context, cfg *rest.Config) *Stores {
    dynamicClient := dynamic.NewForConfigOrDie(cfg)
    return &Stores{
        APIExposureStore:     NewOrDie[*apiv1.ApiExposure](ctx, dynamicClient, ...),
        APISubscriptionStore: NewOrDie[*apiv1.ApiSubscription](ctx, dynamicClient, ...),
        ApplicationStore:     NewOrDie[*applicationv1.Application](ctx, dynamicClient, ...),
        ZoneStore:            NewOrDie[*adminv1.Zone](ctx, dynamicClient, ...),
        ApprovalStore:        NewOrDie[*approvalv1.Approval](ctx, dynamicClient, ...),
    }
}
```

---

## 5. Security Model

### 5.1 OAuth2 Scopes

The OpenAPI spec defines scopes at: `tardis:{clientType}:{accessType}`

| Client Type | Read | All | Obfuscated |
|------------|------|-----|------------|
| admin | `tardis:admin:read` | `tardis:admin:all` | — |
| supervisor | `tardis:supervisor:read` | `tardis:supervisor:all` | `tardis:supervisor:obfuscated` |
| hub | `tardis:hub:read` | `tardis:hub:all` | `tardis:hub:obfuscated` |
| team | `tardis:team:read` | `tardis:team:all` | `tardis:team:obfuscated` |
| user | `tardis:user:read` | `tardis:user:all` | `tardis:user:obfuscated` |

**Default scope:** `tardis:user:read` — lowest privilege, sufficient for all read endpoints.

### 5.2 Security Templates

The `applicationId` path parameter requires different security templates than rover-server's `resourceId`:

```go
var securityTemplates = map[security.ClientType]security.ComparisonTemplates{
    security.ClientTypeTeam: {
        ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}--",
        UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
        MatchType:         security.MatchTypePrefix,
    },
    security.ClientTypeGroup: {
        ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
        UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
        MatchType:         security.MatchTypePrefix,
    },
    security.ClientTypeAdmin: {
        ExpectedTemplate:  "{{ .B.Environment }}--",
        UserInputTemplate: "{{ .B.Environment }}--{{ .P.Applicationid }}",
        MatchType:         security.MatchTypePrefix,
    },
}
```

> ⚠️ **Issue:** The `WithPathParamKey` must be set to `"applicationId"` (not `"resourceId"`). The template references `{{ .P.Applicationid }}` — the casing depends on how the security middleware normalizes path param keys. **This needs verification against the middleware implementation.**

### 5.3 Obfuscated Access

For `obfuscated` access type, sensitive fields (e.g., security credentials, upstream URLs) should be masked. This is a new requirement not present in rover-server and needs careful design:

- The `BusinessContext` provides `AccessType` (read/all/obfuscated)
- The mapper layer should check `AccessType` and mask fields accordingly
- Masked fields return a fixed placeholder (e.g., `"***"`)

---

## 6. Pagination Strategy

### 6.1 The Mismatch

| OpenAPI spec (stargate) | ObjectStore (common-server) |
|------------------------|----------------------------|
| Offset/limit pagination | Cursor-based pagination |
| `offset` + `limit` query params | `cursor` + `limit` in ListOpts |
| `Paging` object with `total`, `page`, `last_page` | `ListResponse` with `next` cursor link |
| `X-Total-Count`, `X-Result-Count` headers | Not natively supported |
| HATEOAS `_links` (first/self/last/prev/next) | Only `self` and `next` |

### 6.2 Proposed Solution: In-Memory Pagination Adapter

Since spy-server is read-only and resources are scoped per application (typically tens to low hundreds of items), we can safely use an **in-memory pagination adapter**:

```go
// fetchAll retrieves all items from the store matching the given opts
// by following cursor-based pagination to completion.
func fetchAll[T store.Object](ctx context.Context, s store.ObjectStore[T], opts store.ListOpts) ([]T, error) {
    var all []T
    opts.Limit = 500 // fetch in large batches
    for {
        resp, err := s.List(ctx, opts)
        if err != nil {
            return nil, err
        }
        all = append(all, resp.Items...)
        if resp.Links.Next == "" {
            break
        }
        opts.Cursor = extractCursor(resp.Links.Next)
    }
    return all, nil
}

// PaginatedResult holds the offset/limit paginated slice + metadata
type PaginatedResult[T any] struct {
    Items  []T
    Paging api.Paging
    Links  api.Links
}

// paginate applies offset/limit to a fully loaded slice
func paginate[T any](items []T, offset, limit *int) PaginatedResult[T] {
    total := len(items)
    off := derefOrDefault(offset, 0)
    lim := derefOrDefault(limit, 20)

    if off > total { off = total }
    end := off + lim
    if end > total { end = total }

    page := off / lim
    lastPage := (total - 1) / lim
    if total == 0 { lastPage = 0 }

    return PaginatedResult[T]{
        Items: items[off:end],
        Paging: api.Paging{
            Total:    total,
            Page:     page,
            LastPage: lastPage,
        },
        Links: buildLinks(off, lim, total),
    }
}
```

### 6.3 Why This Is Acceptable

- **Application-scoped data is bounded.** A single application typically has <100 exposures/subscriptions.
- **Read-only.** No write amplification risk.
- **Correctness.** Offset/limit semantics are exact — no cursor invalidation issues.
- **Simplicity.** Avoids building a custom offset-aware store adapter.

### 6.4 Response Headers

The server handler must also set:
- `X-Total-Count` → total number of items matching the filter
- `X-Result-Count` → number of items in the current page

```go
c.Set("X-Total-Count", strconv.Itoa(result.Paging.Total))
c.Set("X-Result-Count", strconv.Itoa(len(result.Items)))
```

---

## 7. Deprecated Endpoints

All POST, PUT, DELETE endpoints are marked `deprecated: true` in the OpenAPI spec.

### 7.1 Strategy

Return **HTTP 410 Gone** with an RFC 7807 Problem Details body pointing users to the Rover API:

```go
func (s *Server) registerDeprecatedRoutes(router fiber.Router, checkAccess fiber.Handler) {
    deprecatedHandler := func(c *fiber.Ctx) error {
        return server.ReturnWithProblem(c, problems.Gone(
            "This endpoint is deprecated. Use the Rover API for write operations.",
        ))
    }

    // ApiExposure write endpoints
    router.Post("/applications/:applicationId/apiexposures", checkAccess, deprecatedHandler)
    router.Put("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, deprecatedHandler)
    router.Delete("/applications/:applicationId/apiexposures/:apiExposureName", checkAccess, deprecatedHandler)

    // ApiSubscription write endpoints
    router.Post("/applications/:applicationId/apisubscriptions", checkAccess, deprecatedHandler)
    router.Put("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, deprecatedHandler)
    router.Delete("/applications/:applicationId/apisubscriptions/:apiSubscriptionName", checkAccess, deprecatedHandler)
    router.Post("/applications/:applicationId/apisubscriptions/:apiSubscriptionName/approve", checkAccess, deprecatedHandler)
}
```

### 7.2 Alternative Considered

- **501 Not Implemented**: Less semantically correct — the endpoints _were_ implemented, they're now deprecated.
- **301 Redirect to Rover API**: Would require knowing the equivalent Rover URL, which has different resource IDs.
- **✅ 410 Gone**: Correct HTTP semantics — the resource at this URL is permanently gone.

---

## 8. Potential Issues & Risks

### 🔴 Critical

#### Issue 1: Pagination Model Mismatch
**Problem:** The OpenAPI spec requires offset/limit pagination with `total` counts, but `ObjectStore` only supports cursor-based pagination with no total count.  
**Impact:** Cannot directly use store pagination; need in-memory adapter.  
**Mitigation:** In-memory `fetchAll` + slice-based pagination (see §6). Acceptable because data is application-scoped and bounded.  
**Risk:** If an application has thousands of exposures/subscriptions, memory usage could spike. Consider adding a hard cap (e.g., max 1000 items per application).

#### Issue 2: Missing `variant` Field in CRD
**Problem:** The OpenAPI spec defines a `variant` field on ApiExposure with enum values `DEFAULT`, `MCP`, `TELECONTEXTMCP`. The `ApiExposure` CRD type has no such field.  
**Impact:** Cannot populate the `variant` response field from existing CRD data.  
**Mitigation Options:**
1. **Extend the CRD** to add a `Variant` field to ApiExposure spec (preferred, but requires CRD migration)
2. **Use a label/annotation** (e.g., `cp.ei.telekom.de/variant`) to store variant info
3. **Default to `"DEFAULT"`** until the CRD is extended
**Recommendation:** Option 3 as short-term, Option 1 as follow-up.

### 🟡 Important

#### Issue 3: N+1 Query Problem for ApiSubscription
**Problem:** Each `ApiSubscriptionResponse` requires lookups to Application, Zone, and Approval stores.  
**Impact:** Listing 50 subscriptions could trigger up to 150 additional store lookups.  
**Mitigation:**
- Use a **batch-resolve pattern**: collect all unique Zone/Approval/Application references first, resolve them in batch, then map.
- The in-memory store makes individual lookups O(1) (hash table), so this is more a code complexity issue than a performance issue.

#### Issue 4: Security Template Path Param Casing
**Problem:** The `CheckAccess` middleware uses `{{ .P.<ParamName> }}` templates. The Fiber param key is `applicationId` but the template engine may capitalize differently (e.g., `Applicationid` vs `ApplicationId`).  
**Impact:** Security check could silently pass or fail if casing is wrong.  
**Mitigation:** Test the exact casing used by `security.WithPathParamKey("applicationId")` and match templates accordingly. Check the middleware source to confirm normalization rules.

#### Issue 5: Sort Parameter Mapping
**Problem:** The OpenAPI spec supports `sort=name` (ascending by name). The store uses JSON-path-based sorters like `metadata.name:asc`.  
**Impact:** Need a translation from API sort values to store sort paths.  
**Mitigation:** Map `"name"` → `"metadata.name:asc"` in the controller. Only "name" is supported per the spec.

#### Issue 6: Application-Scoped Label Filtering  
**Problem:** Resources need to be filtered by `cp.ei.telekom.de/application=<appName>` label. The store filter uses JSON path `metadata.labels.cp\\.ei\\.telekom\\.de/application`.  
**Impact:** The dot-escaped label key path may not work correctly with the store's filter implementation.  
**Mitigation:** Verify that the in-memory store's filter supports escaped dots in JSON paths. If not, use the `Prefix` mechanism to filter by namespace and then filter by label in-memory after fetching.

### 🟢 Minor

#### Issue 7: GatewayUrl Construction Requires Zone Lookup
**Problem:** The gateway URL is `Zone.Status.Links.Url + ApiBasePath`. The Zone reference is an ObjectRef on the subscription/exposure.  
**Impact:** Need to resolve the Zone by namespace/name from the ObjectRef.  
**Mitigation:** Straightforward store.Get() call. Cache Zone lookups per request since many subscriptions may share the same zone.

#### Issue 8: Problems Package May Lack `Gone` (410) Factory
**Problem:** The common-server `problems` package may not have a factory for HTTP 410 Gone status.  
**Impact:** Deprecated endpoints need a 410 response.  
**Mitigation:** Add a `problems.Gone()` factory or use `problems.NewProblem()` directly with status 410.

#### Issue 9: `GetApiExposureSubscriptions` Filtering
**Problem:** The endpoint `GET /applications/{applicationId}/apiexposures/{apiExposureName}/apisubscriptions` lists subscriptions for a specific exposure. There's no direct CRD link from ApiSubscription to ApiExposure — the link is via `apiBasePath`.  
**Impact:** Need to match subscriptions by `apiBasePath` matching the exposure's `apiBasePath`.  
**Mitigation:** Fetch the exposure first, get its `apiBasePath`, then filter subscriptions by `spec.apiBasePath == exposure.spec.apiBasePath`.

---

## 9. Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Use `oapi-codegen` for types + embedded spec only | Matches rover-server pattern; hand-written handlers give control over pagination and error handling |
| D2 | In-memory pagination adapter (fetchAll + slice) | Offset/limit required by spec; cursor-to-offset conversion is impractical; data is bounded per application |
| D3 | Return 410 Gone for deprecated write endpoints | Correct HTTP semantics; clear message directing to Rover API |
| D4 | No in-mappers needed | Read-only server; no request body parsing for resource creation/update |
| D5 | Batch-resolve cross-resource references | Avoids N+1 for subscription listings; in-memory store makes individual lookups fast but batch is cleaner |
| D6 | Default `variant` to `"DEFAULT"` until CRD is extended | Unblocks development; CRD extension is a separate work item |
| D7 | Exclude ConsumeRouteStore from initial design | Failover URLs can be constructed from Zone references; add if needed |
| D8 | Security templates use `applicationId` as path param key | Differs from rover-server's `resourceId`; format is `<group>--<team>--<appName>` which aligns with existing prefix-matching |
