<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Event Resources — Design Document

> **Version:** 1.0  
> **Date:** 2025-03-30  
> **Status:** Accepted  
> **Component:** `spy-server/`  
> **Depends on:** [000_initial design](../000_initial/design.md), [001_application design](../001_application/design.md)

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

This document describes the design for adding **Event** resource endpoints to the spy-server. The Event domain has three resource types:

- **EventExposure** — Scoped under `/applications/{applicationId}/eventexposures`
- **EventSubscription** — Scoped under `/applications/{applicationId}/eventsubscriptions`
- **EventType** — Global resource at `/eventtypes` (not scoped under any application)

The EventExposure and EventSubscription endpoints follow the same pattern as ApiExposure and ApiSubscription from [000_initial](../000_initial/design.md). EventType is a new pattern — a **global resource** with no `applicationId` path parameter and **no `checkAccess` middleware**.

### Key Facts

| Aspect | Value |
|--------|-------|
| OpenAPI spec | `api/event-openapi.yaml` |
| API base path | Merged into `/stargate/v2` via `uber-openapi.yaml` |
| Resource scopes | `/applications/{applicationId}/eventexposures`, `/applications/{applicationId}/eventsubscriptions`, `/eventtypes` |
| Active endpoints | 11 GET endpoints |
| Deprecated endpoints | 7 POST/PUT/DELETE endpoints |
| New controller interfaces | `EventExposureController`, `EventSubscriptionController`, `EventTypeController` |
| CRD sources | `event.cp.ei.telekom.de/v1` → `EventExposure`, `EventSubscription`, `EventType` |

### Endpoint Summary

| # | Method | Path | Controller | Notes |
|---|--------|------|-----------|-------|
| 1 | GET | `/applications/{applicationId}/eventexposures` | EventExposure.GetAll | Scoped list |
| 2 | GET | `/applications/{applicationId}/eventexposures/{eventExposureName}` | EventExposure.Get | Single |
| 3 | GET | `/applications/{applicationId}/eventexposures/{eventExposureName}/status` | EventExposure.GetStatus | Status |
| 4 | GET | `/applications/{applicationId}/eventexposures/{eventExposureName}/eventsubscriptions` | EventExposure.GetSubscriptions | Cross-namespace |
| 5 | GET | `/applications/{applicationId}/eventsubscriptions` | EventSubscription.GetAll | Scoped list |
| 6 | GET | `/applications/{applicationId}/eventsubscriptions/{eventSubscriptionName}` | EventSubscription.Get | Single |
| 7 | GET | `/applications/{applicationId}/eventsubscriptions/{eventSubscriptionName}/status` | EventSubscription.GetStatus | Status |
| 8 | GET | `/eventtypes` | EventType.GetAll | **Global** list |
| 9 | GET | `/eventtypes/{eventTypeName}` | EventType.Get | **Global** single |
| 10 | GET | `/eventtypes/{eventTypeName}/status` | EventType.GetStatus | **Global** status |

---

## 2. Scope

### In Scope

- All 11 GET endpoints listed above
- 7 deprecated POST/PUT/DELETE endpoints returning 410 Gone
- CRD → API response mapping for all three resource types
- Cross-resource lookups (Application, Approval stores)
- EventType name resolution (dot-to-hyphen conversion via `MakeEventTypeName`)
- Security integration via `checkAccess` + `PrefixFromContext` for application-scoped endpoints
- EventType routes without `checkAccess` (global resources)

### Out of Scope (Deferred)

- **Obfuscated access type:** Field masking for `obfuscated` scope is deferred (same as 000_initial).
- **EventType security scoping:** EventType routes currently have no `checkAccess` middleware. If security scoping is needed (e.g., to restrict which EventTypes a team can see), this requires a separate design discussion since EventTypes are global resources without an `applicationId` path parameter.

---

## 3. Architecture

### 3.1 Data Flow

EventExposure and EventSubscription follow the identical layered architecture from 000_initial:

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
│  (event*_server.go) │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Controller Layer   │  ← Orchestrate: parse applicationId, query store, invoke mapper
│  (event*.go)        │
└─────────┬───────────┘
          │
          ▼
┌──────────┴──────────┐
│                     │
▼                     ▼
┌───────────┐   ┌───────────┐
│  Mapper   │   │   Store   │  ← EventExposureStore, EventSubscriptionStore, EventTypeStore
│ (event*/  │   │           │    (+ ApplicationStore, ApprovalStore for cross-refs)
│  out.go)  │   │           │
└───────────┘   └───────────┘
```

EventType is slightly different — the middleware chain skips `checkAccess`:

```
HTTP Request
    │
    ▼
┌─────────────────────┐
│   Fiber Middleware   │  ← JWT, BusinessContext, OpenAPI validation (NO checkAccess)
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│   Server Layer      │
│ (eventtype_server)  │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Controller Layer   │  ← Full table scan (no prefix scoping)
│  (eventtype.go)     │
└─────────┬───────────┘
          │
          ▼
┌──────────┴──────────┐
│                     │
▼                     ▼
┌───────────┐   ┌───────────┐
│  Mapper   │   │   Store   │  ← EventTypeStore only
│(eventtype)│   │           │
└───────────┘   └───────────┘
```

### 3.2 Key Differences from ApiExposure/ApiSubscription

| Aspect | ApiExposure / ApiSubscription | EventExposure / EventSubscription | EventType |
|--------|-------------------------------|-----------------------------------|-----------|
| Path pattern | `/applications/{appId}/api*` | `/applications/{appId}/event*` | `/eventtypes` |
| Has `applicationId` | Yes | Yes | **No** |
| `checkAccess` middleware | Yes | Yes | **No** |
| Scoping (list) | Namespace prefix + app label | Namespace prefix + app label | **None** (full scan) |
| Cross-resource lookups | Zone, Approval, Application | Application, Approval | **None** |
| Name construction (Get) | `<appName>--<subresourceName>` | EventExposure: direct name; EventSubscription: `<appName>--<subresourceName>` | Name matching via `MakeEventTypeName` |
| GetSubscriptions variant | Matches by `apiBasePath` | Matches by `eventType` | N/A |

### 3.3 EventExposure vs EventSubscription Name Conventions

A notable difference exists in how Get endpoints resolve CRD names:

- **EventExposure.Get**: Uses the `eventExposureName` directly as the store key (no prefix with appName)
- **EventSubscription.Get**: Constructs `<appName>--<eventSubscriptionName>` as the full store key
- **ApiExposure.Get**: Constructs `<appName>--<apiExposureName>` as the full store key
- **ApiSubscription.Get**: Constructs `<appName>--<apiSubscriptionName>` as the full store key

This difference reflects how the CRDs are named in the cluster. EventExposure names are already unique within a namespace, while EventSubscription names are prefixed with the application name to ensure uniqueness.

---

## 4. Layer-by-Layer Design

### 4.1 Server Layer (HTTP Handlers)

#### 4.1.1 Controller Interfaces

Added to `internal/server/server.go`:

```go
// EventExposureController defines the read-only operations for EventExposure resources.
type EventExposureController interface {
    Get(ctx context.Context, applicationId, eventExposureName string) (api.EventExposureResponse, error)
    GetAll(ctx context.Context, applicationId string, params api.GetAllEventExposuresParams) (*api.EventExposureListResponse, error)
    GetStatus(ctx context.Context, applicationId, eventExposureName string) (api.ResourceStatusResponse, error)
    GetSubscriptions(ctx context.Context, applicationId, eventExposureName string, params api.GetAllExposureEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error)
}

// EventSubscriptionController defines the read-only operations for EventSubscription resources.
type EventSubscriptionController interface {
    Get(ctx context.Context, applicationId, eventSubscriptionName string) (api.EventSubscriptionResponse, error)
    GetAll(ctx context.Context, applicationId string, params api.GetAllEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error)
    GetStatus(ctx context.Context, applicationId, eventSubscriptionName string) (api.ResourceStatusResponse, error)
}

// EventTypeController defines the read-only operations for EventType resources.
type EventTypeController interface {
    Get(ctx context.Context, eventTypeName string) (api.EventTypeResponse, error)
    GetAll(ctx context.Context, params api.GetAllEventTypesParams) (*api.EventTypeListResponse, error)
    GetStatus(ctx context.Context, eventTypeName string) (api.ResourceStatusResponse, error)
}
```

#### 4.1.2 Server Struct Extension

```go
type Server struct {
    Config             *config.ServerConfig
    Log                logr.Logger
    ApiExposures       ApiExposureController
    ApiSubscriptions   ApiSubscriptionController
    Applications       ApplicationController
    EventExposures     EventExposureController        // ← NEW
    EventSubscriptions EventSubscriptionController     // ← NEW
    EventTypes         EventTypeController             // ← NEW
}
```

#### 4.1.3 Handler Pattern

EventExposure and EventSubscription handlers follow the exact same pattern as ApiExposure/ApiSubscription:

```go
// GetAllEventExposures handles GET /applications/:applicationId/eventexposures
func (s *Server) GetAllEventExposures(c *fiber.Ctx) error {
    applicationId := c.Params("applicationId")
    params := api.GetAllEventExposuresParams{}
    if err := c.QueryParser(&params); err != nil {
        return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
    }

    resp, err := s.EventExposures.GetAll(c.UserContext(), applicationId, params)
    if err != nil {
        return cserver.ReturnWithProblem(c, nil, err)
    }

    c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
    c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
    return c.JSON(resp)
}
```

EventType handlers differ — no `applicationId` parameter:

```go
// GetAllEventTypes handles GET /eventtypes
func (s *Server) GetAllEventTypes(c *fiber.Ctx) error {
    params := api.GetAllEventTypesParams{}
    if err := c.QueryParser(&params); err != nil {
        return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
    }

    resp, err := s.EventTypes.GetAll(c.UserContext(), params)
    if err != nil {
        return cserver.ReturnWithProblem(c, nil, err)
    }

    c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
    c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
    return c.JSON(resp)
}
```

#### 4.1.4 Route Registration

Added to `RegisterRoutes`:

```go
// EventExposure routes (read-only)
s.Log.Info("Registering eventexposure routes")
router.Get("/applications/:applicationId/eventexposures", checkAccess, s.GetAllEventExposures)
router.Get("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, s.GetEventExposure)
router.Get("/applications/:applicationId/eventexposures/:eventExposureName/status", checkAccess, s.GetEventExposureStatus)
router.Get("/applications/:applicationId/eventexposures/:eventExposureName/eventsubscriptions", checkAccess, s.GetEventExposureSubscriptions)

// EventSubscription routes (read-only)
s.Log.Info("Registering eventsubscription routes")
router.Get("/applications/:applicationId/eventsubscriptions", checkAccess, s.GetAllEventSubscriptions)
router.Get("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, s.GetEventSubscription)
router.Get("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName/status", checkAccess, s.GetEventSubscriptionStatus)

// EventType routes (read-only, global — not scoped under /applications)
s.Log.Info("Registering eventtype routes")
router.Get("/eventtypes", s.GetAllEventTypes)
router.Get("/eventtypes/:eventTypeName", s.GetEventType)
router.Get("/eventtypes/:eventTypeName/status", s.GetEventTypeStatus)
```

> **Note:** EventType routes do NOT use `checkAccess` middleware. EventTypes are global resources visible to all authenticated callers. See [§5 Security Model](#5-security-model) for rationale.

### 4.2 Controller Layer

#### 4.2.1 EventExposure Controller

##### GetAll — Application-Scoped List

The `GetAll` method follows the same pattern as `ApiExposure.GetAll`: filter by namespace prefix and application label.

```go
func (c *eventExposureController) GetAll(ctx context.Context, applicationId string, params api.GetAllEventExposuresParams) (*api.EventExposureListResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    if err != nil {
        return nil, err
    }

    listOpts := store.NewListOpts()
    listOpts.Prefix = appInfo.Namespace + "/"
    listOpts.Filters = append(listOpts.Filters, store.Filter{
        Path:  mapper.ApplicationLabelPath,
        Op:    store.OpEqual,
        Value: appInfo.AppName,
    })

    items, err := pagination.FetchAll(ctx, c.stores.EventExposureStore, listOpts)
    // ... map + paginate
}
```

##### Get — Single EventExposure

Uses the `eventExposureName` directly (no `<appName>--` prefix):

```go
func (c *eventExposureController) Get(ctx context.Context, applicationId, eventExposureName string) (api.EventExposureResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    // ...
    exposure, err := c.stores.EventExposureStore.Get(ctx, appInfo.Namespace, eventExposureName)
    // ...
    if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
        return api.EventExposureResponse{}, err
    }
    return eventexposuremapper.MapResponseWithResourceName(ctx, exposure, c.stores), nil
}
```

##### GetSubscriptions — Cross-Namespace Event Subscriptions

This endpoint returns all EventSubscriptions that subscribe to the same `eventType` as the specified EventExposure, regardless of which application or namespace they belong to:

```go
func (c *eventExposureController) GetSubscriptions(ctx context.Context, applicationId, eventExposureName string, params api.GetAllExposureEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error) {
    // 1. Fetch and verify the exposure
    exposure, err := c.stores.EventExposureStore.Get(ctx, appInfo.Namespace, eventExposureName)
    // ...

    // 2. Fetch ALL event subscriptions (cross-namespace, no prefix/app filter)
    listOpts := store.NewListOpts()
    allSubs, err := pagination.FetchAll(ctx, c.stores.EventSubscriptionStore, listOpts)

    // 3. Filter by matching eventType
    targetEventType := exposure.Spec.EventType
    for _, sub := range allSubs {
        if sub.Spec.EventType == targetEventType {
            matchingSubs = append(matchingSubs, ...)
        }
    }
    // ... paginate
}
```

> **Note:** The cross-namespace fetch (no prefix filter) is intentional — subscribers come from different teams/applications. This mirrors the pattern from `ApiExposure.GetSubscriptions` which matches by `apiBasePath`.

#### 4.2.2 EventSubscription Controller

##### GetAll — Application-Scoped List

Identical pattern to `ApiSubscription.GetAll`:

```go
func (c *eventSubscriptionController) GetAll(ctx context.Context, applicationId string, params api.GetAllEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    // ...
    listOpts := store.NewListOpts()
    listOpts.Prefix = appInfo.Namespace + "/"
    listOpts.Filters = append(listOpts.Filters, store.Filter{
        Path:  mapper.ApplicationLabelPath,
        Op:    store.OpEqual,
        Value: appInfo.AppName,
    })
    // ... fetch, map, paginate
}
```

##### Get — Single EventSubscription

Constructs full name as `<appName>--<eventSubscriptionName>`:

```go
func (c *eventSubscriptionController) Get(ctx context.Context, applicationId, eventSubscriptionName string) (api.EventSubscriptionResponse, error) {
    appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
    eventSubscriptionFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventSubscriptionName)
    sub, err := c.stores.EventSubscriptionStore.Get(ctx, appInfo.Namespace, eventSubscriptionFullName)
    // ... verify label, map
}
```

#### 4.2.3 EventType Controller

##### GetAll — Global List (No Prefix Scoping)

EventTypes are global resources. The `GetAll` method fetches all EventTypes without any namespace or security prefix scoping:

```go
func (c *eventTypeController) GetAll(ctx context.Context, params api.GetAllEventTypesParams) (*api.EventTypeListResponse, error) {
    listOpts := store.NewListOpts()
    items, err := pagination.FetchAll(ctx, c.stores.EventTypeStore, listOpts)
    // ... map + paginate with basePath "/eventtypes"
}
```

##### Get — Single EventType (Full Scan + Name Matching)

EventType names in the store may not directly match the API name (dot-to-hyphen conversion). The `Get` method performs a full table scan and matches by name or computed name:

```go
func (c *eventTypeController) Get(ctx context.Context, eventTypeName string) (api.EventTypeResponse, error) {
    listOpts := store.NewListOpts()
    allTypes, err := pagination.FetchAll(ctx, c.stores.EventTypeStore, listOpts)
    // ...
    for _, et := range allTypes {
        if et.GetName() == eventTypeName || eventv1.MakeEventTypeName(et.Spec.Type) == eventTypeName {
            return eventtypemapper.MapResponse(et), nil
        }
    }
    return api.EventTypeResponse{}, problems.NotFound(eventTypeName)
}
```

> **Note:** The full table scan is acceptable because EventTypes are a small, bounded set (typically tens to low hundreds). See [§8 Issue 3](#issue-3-eventtype-full-table-scan) for discussion.

### 4.3 Mapper Layer

#### 4.3.1 EventExposure Out-Mapper

Located at `internal/mapper/eventexposure/out.go`:

```go
func MapResponse(ctx context.Context, in *eventv1.EventExposure, stores *sstore.Stores) api.EventExposureResponse {
    resp := api.EventExposureResponse{
        Name:       in.GetName(),
        EventType:  in.Spec.EventType,
        Visibility: toAPIVisibility(in.Spec.Visibility),
        Approval: api.EventApproval{
            Strategy:     toAPIApprovalStrategy(in.Spec.Approval.Strategy),
            TrustedTeams: in.Spec.Approval.TrustedTeams,
        },
        Zone:                   in.Spec.Zone.Name,
        Active:                 in.Status.Active,
        CallbackURL:            in.Status.CallbackURL,
        SseUrls:                in.Status.SseURLs,
        AdditionalPublisherIds: in.Spec.AdditionalPublisherIds,
        Status:                 status.MapStatus(in.GetConditions(), in.GetGeneration()),
    }
    mapScopes(in, &resp)
    mapTeamAndApplication(ctx, in, &resp, stores.ApplicationStore)
    return resp
}
```

##### Field Mapping Summary — EventExposure

| API field | CRD source | Notes |
|-----------|-----------|-------|
| `name` | `obj.GetName()` or `MakeResourceName(obj)` | Direct name or `<group>--<team>--<name>` for lists |
| `eventType` | `obj.Spec.EventType` | Direct |
| `visibility` | `obj.Spec.Visibility` | Enum mapping: `world`→`World`, `zone`→`Zone`, `enterprise`→`Enterprise` |
| `approval.strategy` | `obj.Spec.Approval.Strategy` | Enum mapping: `auto`→`Auto`, `simple`→`Simple`, `fourEyes`→`FourEyes` |
| `approval.trustedTeams` | `obj.Spec.Approval.TrustedTeams` | Direct |
| `zone` | `obj.Spec.Zone.Name` | ObjectRef → name only |
| `active` | `obj.Status.Active` | Direct from status |
| `callbackURL` | `obj.Status.CallbackURL` | Direct from status |
| `sseUrls` | `obj.Status.SseURLs` | Direct from status |
| `additionalPublisherIds` | `obj.Spec.AdditionalPublisherIds` | Direct |
| `scopes[].name` | `obj.Spec.Scopes[].Name` | Direct |
| `scopes[].trigger.selectionFilter.expression` | `obj.Spec.Scopes[].Trigger.SelectionFilter.Expression` | `*apiextensionsv1.JSON` → `map[string]interface{}` (JSON unmarshal) |
| `scopes[].trigger.selectionFilter.attributes` | `obj.Spec.Scopes[].Trigger.SelectionFilter.Attributes` | Direct |
| `scopes[].trigger.responseFilter.paths` | `obj.Spec.Scopes[].Trigger.ResponseFilter.Paths` | Direct |
| `scopes[].trigger.responseFilter.mode` | `obj.Spec.Scopes[].Trigger.ResponseFilter.Mode` | Direct cast |
| `team` | Application store lookup or namespace fallback | Hub from namespace, name/email from Application |
| `application` | Application store lookup or spec fallback | Name from Application or Provider ref |
| `status` | `status.MapStatus(conditions, generation)` | Reuses shared mapper |

##### Enum Mapping — Visibility and ApprovalStrategy

The CRD uses lowercase enum values (`world`, `zone`, `enterprise`, `auto`, `simple`, `fourEyes`) while the API uses PascalCase (`World`, `Zone`, `Enterprise`, `Auto`, `Simple`, `FourEyes`). Explicit switch-case mapping is used:

```go
func toAPIVisibility(v eventv1.Visibility) api.EventVisibility {
    switch v {
    case eventv1.VisibilityWorld:     return api.World
    case eventv1.VisibilityZone:      return api.Zone
    case eventv1.VisibilityEnterprise: return api.Enterprise
    default: return api.EventVisibility(strings.ToUpper(string(v)))
    }
}
```

##### SelectionFilter.Expression Unmarshal

The CRD stores `Expression` as `*apiextensionsv1.JSON` (raw bytes) while the API expects `map[string]interface{}`. The mapper performs JSON unmarshal:

```go
if in.SelectionFilter.Expression != nil && in.SelectionFilter.Expression.Raw != nil {
    var expr map[string]interface{}
    if err := json.Unmarshal(in.SelectionFilter.Expression.Raw, &expr); err == nil {
        out.SelectionFilter.Expression = expr
    }
}
```

#### 4.3.2 EventSubscription Out-Mapper

Located at `internal/mapper/eventsubscription/out.go`:

##### Field Mapping Summary — EventSubscription

| API field | CRD source | Notes |
|-----------|-----------|-------|
| `name` | `obj.GetName()` or `MakeResourceName(obj)` | Direct or `<group>--<team>--<name>` for lists |
| `eventType` | `obj.Spec.EventType` | Direct |
| `zone` | `obj.Spec.Zone.Name` | ObjectRef → name only |
| `scopes` | `obj.Spec.Scopes` | Direct (string slice) |
| `subscriptionId` | `obj.Status.SubscriptionId` | Direct from status |
| `url` | `obj.Status.URL` | Direct from status |
| `delivery.type` | `obj.Spec.Delivery.Type` | Direct cast to API enum |
| `delivery.payload` | `obj.Spec.Delivery.Payload` | Direct cast to API enum |
| `delivery.callback` | `obj.Spec.Delivery.Callback` | Direct |
| `delivery.eventRetentionTime` | `obj.Spec.Delivery.EventRetentionTime` | Direct |
| `delivery.circuitBreakerOptOut` | `obj.Spec.Delivery.CircuitBreakerOptOut` | Direct |
| `delivery.retryableStatusCodes` | `obj.Spec.Delivery.RetryableStatusCodes` | Direct |
| `delivery.redeliveriesPerSecond` | `obj.Spec.Delivery.RedeliveriesPerSecond` | `*int` → `int` (dereference, default 0) |
| `delivery.enforceGetHttpRequestMethodForHealthCheck` | `obj.Spec.Delivery.EnforceGetHttpRequestMethodForHealthCheck` | Direct |
| `trigger` | `obj.Spec.Trigger` | Optional; same EventTrigger mapping as EventExposure |
| `team` | Application store lookup or namespace fallback | Via `Spec.Requestor` ref |
| `application` | Application store lookup or spec fallback | Via `Spec.Requestor` ref |
| `approval` | Approval store lookup | Via `Status.Approval` ref; includes status, decider, comment |
| `status` | `status.MapStatus(conditions, generation)` | Reuses shared mapper |

##### Delivery.RedeliveriesPerSecond Type Mismatch

The CRD stores `RedeliveriesPerSecond` as `*int` while the API expects `int`. The mapper dereferences with a zero default:

```go
if d.RedeliveriesPerSecond != nil {
    out.Delivery.RedeliveriesPerSecond = *d.RedeliveriesPerSecond
}
```

##### Approval Mapping

The EventSubscription's approval status is resolved by looking up the Approval CRD via `Status.Approval` ObjectRef:

```go
func mapApproval(ctx context.Context, in *eventv1.EventSubscription, out *api.EventSubscriptionResponse, approvalStore store.ObjectStore[*approvalv1.Approval]) {
    if in.Status.Approval == nil {
        return
    }
    approval, err := approvalStore.Get(ctx, in.Status.Approval.Namespace, in.Status.Approval.Name)
    if err != nil || approval == nil {
        return  // Best effort — approval may not exist yet
    }
    out.Approval = api.EventSubscriptionApproval{
        Status: string(approval.Spec.State),
    }
    if len(approval.Spec.Decisions) > 0 {
        latest := approval.Spec.Decisions[len(approval.Spec.Decisions)-1]
        out.Approval.Decider = latest.Email
        out.Approval.Comment = latest.Comment
    }
}
```

#### 4.3.3 EventType Out-Mapper

Located at `internal/mapper/eventtype/out.go`. This is the simplest mapper — no cross-resource lookups:

```go
func MapResponse(in *eventv1.EventType) api.EventTypeResponse {
    return api.EventTypeResponse{
        Name:          in.GetName(),
        Type:          in.Spec.Type,
        Version:       in.Spec.Version,
        Description:   in.Spec.Description,
        Specification: in.Spec.Specification,
        Active:        in.Status.Active,
        Status:        status.MapStatus(in.GetConditions(), in.GetGeneration()),
    }
}
```

##### Field Mapping Summary — EventType

| API field | CRD source | Notes |
|-----------|-----------|-------|
| `name` | `obj.GetName()` | CRD name (hyphenated form) |
| `type` | `obj.Spec.Type` | Dotted form (e.g., `de.telekom.eni.quickstart.v1`) |
| `version` | `obj.Spec.Version` | Direct |
| `description` | `obj.Spec.Description` | Direct |
| `specification` | `obj.Spec.Specification` | Direct (URL) |
| `active` | `obj.Status.Active` | Direct from status |
| `status` | `status.MapStatus(conditions, generation)` | Reuses shared mapper |

### 4.4 Store Layer

Three new stores are added to `pkg/store/stores.go`:

```go
type Stores struct {
    // Existing stores
    APIExposureStore     store.ObjectStore[*apiv1.ApiExposure]
    APISubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
    ApplicationStore     store.ObjectStore[*applicationv1.Application]
    ZoneStore            store.ObjectStore[*adminv1.Zone]
    ApprovalStore        store.ObjectStore[*approvalv1.Approval]
    // New event stores
    EventExposureStore     store.ObjectStore[*eventv1.EventExposure]
    EventSubscriptionStore store.ObjectStore[*eventv1.EventSubscription]
    EventTypeStore         store.ObjectStore[*eventv1.EventType]
}
```

The `ApprovalStore` is reused for EventSubscription approval lookups (same as ApiSubscription).

---

## 5. Security Model

### 5.1 Application-Scoped Endpoints (EventExposure, EventSubscription)

These endpoints use the same `checkAccess` middleware as ApiExposure/ApiSubscription. The `applicationId` path parameter is used for security template matching, and the middleware computes a prefix stored in context.

**Important:** The application-scoped list endpoints (`GetAll`) do NOT need `store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)` because they already scope by the full namespace prefix derived from `applicationId`:

```go
listOpts.Prefix = appInfo.Namespace + "/"
```

The `appInfo.Namespace` is constructed from `<env>--<group>--<team>` which is already scoped to the caller's security context (validated by `checkAccess` before the handler runs). The `checkAccess` middleware rejects the request entirely if the caller doesn't have access to the specified `applicationId`.

This is the same pattern used by `ApiExposure.GetAll` and `ApiSubscription.GetAll` — they also only use `appInfo.Namespace + "/"` and do NOT call `EnforcePrefix`.

> **Clarification:** `store.EnforcePrefix(security.PrefixFromContext(ctx))` is needed ONLY for **global** list endpoints where there is no `applicationId` path parameter (e.g., `GET /applications`). When `applicationId` IS present, the `checkAccess` middleware validates that the caller has access to that specific application, making the namespace prefix already secure.

### 5.2 EventType Endpoints (Global, No checkAccess)

EventType routes are registered **without** `checkAccess`:

```go
router.Get("/eventtypes", s.GetAllEventTypes)
router.Get("/eventtypes/:eventTypeName", s.GetEventType)
router.Get("/eventtypes/:eventTypeName/status", s.GetEventTypeStatus)
```

**Rationale:** EventTypes are global metadata (like a catalog of available event types). They are not owned by any specific application or team. Any authenticated user should be able to browse the event type catalog.

The JWT validation and BusinessContext middleware still run (they're applied at the router level before route-specific middleware), so only authenticated users can access these endpoints.

### 5.3 Cross-Namespace Subscription Listing

The `GetSubscriptions` endpoint on EventExposure fetches ALL event subscriptions across namespaces (no prefix filter) to find subscribers matching the exposure's eventType. This is intentionally not security-scoped because:

1. The caller must have access to the exposure's `applicationId` (validated by `checkAccess`)
2. Seeing which teams subscribe to your event type is expected behavior for the provider

---

## 6. Pagination

Same in-memory pagination adapter as 000_initial for all three resource types:

1. `pagination.FetchAll()` drains the store's cursor-based pagination
2. `pagination.Paginate()` applies offset/limit and builds HATEOAS links
3. Server handler sets `X-Total-Count` and `X-Result-Count` headers

The `basePath` for link construction varies:

| Resource | basePath |
|----------|----------|
| EventExposure | `/applications/{applicationId}/eventexposures` |
| EventSubscription | `/applications/{applicationId}/eventsubscriptions` |
| EventType | `/eventtypes` |
| EventExposure's Subscriptions | `/applications/{applicationId}/eventexposures/{eventExposureName}/eventsubscriptions` |

---

## 7. Deprecated Endpoints

Seven deprecated endpoints return HTTP 410 Gone, added to `registerDeprecatedRoutes`:

```go
// EventExposure write endpoints
router.Post("/applications/:applicationId/eventexposures", checkAccess, deprecatedHandler)
router.Put("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, deprecatedHandler)
router.Delete("/applications/:applicationId/eventexposures/:eventExposureName", checkAccess, deprecatedHandler)

// EventSubscription write endpoints
router.Post("/applications/:applicationId/eventsubscriptions", checkAccess, deprecatedHandler)
router.Put("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, deprecatedHandler)
router.Delete("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName", checkAccess, deprecatedHandler)
router.Post("/applications/:applicationId/eventsubscriptions/:eventSubscriptionName/approve", checkAccess, deprecatedHandler)
```

> **Note:** No deprecated endpoints for EventType — the event-openapi.yaml does not define any write operations for EventTypes.

---

## 8. Potential Issues & Risks

### Important

#### Issue 1: Cross-Namespace Full Scan for GetSubscriptions

**Problem:** `EventExposure.GetSubscriptions` fetches ALL EventSubscriptions across all namespaces to find those matching the exposure's eventType.  
**Impact:** If there are many thousands of EventSubscriptions globally, this becomes expensive.  
**Mitigation:** Same approach as `ApiExposure.GetSubscriptions` — the in-memory store makes this O(n) over all subscriptions. If this becomes a performance issue, consider adding a store-level filter for `spec.eventType` or a secondary index.

#### Issue 2: EventType Full Table Scan for Get and GetStatus

**Problem:** Both `EventType.Get` and `EventType.GetStatus` perform a full table scan (`FetchAll`) and iterate to find the matching EventType by name or computed name. This is because EventTypes can live in any namespace and the caller provides the hyphenated name (which may differ from the CRD metadata name).  
**Impact:** Every single-resource GET triggers a full table scan.  
**Mitigation:** EventTypes are a small dataset (typically tens to low hundreds). If the dataset grows:
1. Add a store-level filter for `spec.type` field
2. Build a secondary index mapping `MakeEventTypeName(spec.type)` → namespace/name
3. Store EventTypes in a dedicated namespace to enable prefix-based lookup

#### Issue 3: EventType Global Visibility

**Problem:** EventType endpoints have no `checkAccess` middleware, making all EventTypes visible to all authenticated users.  
**Impact:** No tenant isolation for EventType metadata.  
**Mitigation:** This is intentional for the current design — EventTypes are a global catalog. If tenant isolation is needed, a separate design iteration is required to define how EventTypes are scoped (by zone? by group? by a new label?).

### Minor

#### Issue 4: EventExposure Name Convention

**Problem:** EventExposure uses the `eventExposureName` directly from the path parameter, while EventSubscription and all Api* resources prepend `<appName>--`. This inconsistency could confuse developers.  
**Impact:** No functional impact — the convention matches how CRDs are named in the cluster.  
**Mitigation:** Document the difference clearly. The CRD naming convention is set by the operators and cannot be changed in the server.

#### Issue 5: Shared EventTrigger Mapping Code

**Problem:** Both EventExposure and EventSubscription mappers contain identical `mapEventTrigger` code (JSON unmarshal for SelectionFilter.Expression, ResponseFilter mapping).  
**Impact:** Code duplication.  
**Mitigation:** Consider extracting to a shared `internal/mapper/event/trigger.go` package. Deferred — the duplication is small and the mappers may diverge in future.

---

## 9. Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | EventExposure/EventSubscription follow the same pattern as Api* resources | Consistency; same security model, same scoping, same pagination |
| D2 | EventType routes have no `checkAccess` middleware | EventTypes are global metadata; all authenticated users should see the catalog |
| D3 | EventType.Get uses full table scan + name matching | EventTypes may use dot-to-hyphen name conversion; small dataset makes scan acceptable |
| D4 | EventExposure.Get uses name directly (no `<appName>--` prefix) | CRD naming convention — EventExposure names are already unique within namespace |
| D5 | EventSubscription.Get constructs `<appName>--<name>` full name | CRD naming convention — EventSubscription names are prefixed with app name |
| D6 | GetSubscriptions fetches cross-namespace (no prefix filter) | Subscribers come from different teams; matching by eventType is the correct semantic |
| D7 | Application-scoped GetAll does NOT need EnforcePrefix | `checkAccess` validates the applicationId; namespace prefix from appInfo is already secure |
| D8 | Shared EventTrigger mapping code is duplicated (not extracted) | Small scope; extraction deferred to avoid premature abstraction |
| D9 | Three new stores (EventExposure, EventSubscription, EventType) + reuse ApprovalStore | ApprovalStore is shared with ApiSubscription for approval resolution |
| D10 | Add deprecated event write endpoints to existing `registerDeprecatedRoutes` | Consistent with 000_initial pattern; no EventType write deprecations needed |
