<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0    
-->

# Spy Server

The Spy Server (also known as the **Stargate API**) is a read-only HTTP API for the TARDIS Control Plane. It provides a stable, user-facing REST interface for querying the state of Applications, ApiExposures, ApiSubscriptions, EventExposures, EventSubscriptions, and EventTypes.

All data is sourced directly from Kubernetes Custom Resources (CRDs) using in-memory stores backed by Kubernetes informers. The server does not perform any write operations on the cluster. Legacy write endpoints are retained for backward compatibility but return `HTTP 410 Gone`, directing clients to the [Rover API](../rover-server/) for mutations.

## Architecture

```
                          ┌──────────────────┐
                          │    API Clients    │
                          │  (OAuth2 / JWT)   │
                          └────────┬─────────┘
                                   │ HTTPS
                          ┌────────▼─────────┐
                          │    Spy Server     │
                          │   (GoFiber HTTP)  │
                          │                   │
                          │  ┌─────────────┐  │
                          │  │  Security    │  │  JWT validation, business
                          │  │  Middleware  │  │  context, scope-based ACL
                          │  └──────┬──────┘  │
                          │  ┌──────▼──────┐  │
                          │  │  OpenAPI     │  │  Request validation against
                          │  │  Validator   │  │  the merged OpenAPI spec
                          │  └──────┬──────┘  │
                          │  ┌──────▼──────┐  │
                          │  │ Controllers  │  │  Read-only business logic
                          │  └──────┬──────┘  │
                          │  ┌──────▼──────┐  │
                          │  │   Mappers    │  │  CRD → API response mapping
                          │  └──────┬──────┘  │
                          │  ┌──────▼──────┐  │
                          │  │ In-Memory    │  │  BadgerDB stores populated
                          │  │ Stores       │  │  by K8s informers
                          │  └──────┬──────┘  │
                          └─────────┼─────────┘
                                    │ watch
                  ┌─────────────────┼─────────────────┐
                  │                 │                  │
          ┌───────▼──────┐  ┌──────▼───────┐  ┌──────▼───────┐
          │ ApiExposure   │  │ Application  │  │ EventType    │
          │ ApiSubscript. │  │ Zone         │  │ EventExposure│
          │               │  │ Approval     │  │ EventSubscr. │
          └───────────────┘  └──────────────┘  └──────────────┘
                    Kubernetes Custom Resources
```

The server watches 8 types of Custom Resources across 5 API groups:

| CRD | API Group | Purpose |
|-----|-----------|---------|
| `ApiExposure` | `api.cp.ei.telekom.de` | APIs exposed on the gateway |
| `ApiSubscription` | `api.cp.ei.telekom.de` | Subscriptions to exposed APIs |
| `Application` | `application.cp.ei.telekom.de` | Registered applications |
| `Zone` | `admin.cp.ei.telekom.de` | Environment zones (used for gateway URL resolution) |
| `Approval` | `approval.cp.ei.telekom.de` | Subscription approval status |
| `EventExposure` | `event.cp.ei.telekom.de` | Published event types |
| `EventSubscription` | `event.cp.ei.telekom.de` | Event subscriptions |
| `EventType` | `event.cp.ei.telekom.de` | Registry of known event types |

## Usage

### Security

All endpoints (except health probes and EventType listing) are protected by OAuth2 JWT authentication. The security middleware performs three steps:

1. **JWT Validation** -- Verifies the token signature and checks against trusted issuers. Optionally validates tokens against an external LMS (License Management Service).
2. **Business Context Extraction** -- Extracts the caller's environment, group, team, and scope from the JWT claims.
3. **Access Control** -- Enforces scope-based access using Go template matching on the `applicationId` path parameter.

#### Scopes

The API uses hierarchical OAuth2 scopes with a `tardis:` prefix. Each scope level grants progressively wider access:

| Scope | Access Level |
|-------|-------------|
| `tardis:user:read` | Read access to own team's resources (default) |
| `tardis:user:all` | Full access to own team's resources |
| `tardis:user:obfuscated` | Read access with sensitive fields redacted |
| `tardis:team:read` | Read access to own team's resources |
| `tardis:team:all` | Full access to own team's resources |
| `tardis:team:obfuscated` | Read access with sensitive fields redacted |
| `tardis:hub:read` | Read access to all resources in a hub/group |
| `tardis:hub:all` | Full access to all resources in a hub/group |
| `tardis:hub:obfuscated` | Hub-wide read access with sensitive fields redacted |
| `tardis:admin:read` | Read access to all resources across environments |
| `tardis:admin:all` | Full access to all resources |
| `tardis:admin:obfuscated` | Admin read access with sensitive fields redacted |
| `tardis:supervisor:read` | Admin-level read access (deprecated alias) |

Access control is enforced by matching the caller's identity against the `applicationId` path parameter using prefix matching:

- **Team clients** -- `<env>--<group>--<team>--` must be a prefix of `<env>--<applicationId>`
- **Group/Hub clients** -- `<env>--<group>--` must be a prefix of `<env>--<applicationId>`
- **Admin clients** -- `<env>--` must be a prefix of `<env>--<applicationId>`

### Endpoints / Resources

The base URL for the API is `https://api.telekom.de/stargate/v2`.

All list endpoints support pagination via `offset`, `limit`, and `sort` query parameters. Responses include `X-Total-Count` and `X-Result-Count` headers, plus a `paging` object with `_links` for HATEOAS navigation.

#### Health Probes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe |

#### Applications

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/applications` | List all applications visible to the caller |
| `GET` | `/applications/{applicationId}` | Get a specific application |
| `GET` | `/applications/{applicationId}/status` | Get application status |

#### API Exposures

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/applications/{applicationId}/apiexposures` | List all API exposures for an application |
| `GET` | `/applications/{applicationId}/apiexposures/{apiExposureName}` | Get a specific API exposure |
| `GET` | `/applications/{applicationId}/apiexposures/{apiExposureName}/status` | Get exposure status |
| `GET` | `/applications/{applicationId}/apiexposures/{apiExposureName}/apisubscriptions` | List all subscriptions to this exposure |

#### API Subscriptions

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/applications/{applicationId}/apisubscriptions` | List all API subscriptions for an application |
| `GET` | `/applications/{applicationId}/apisubscriptions/{apiSubscriptionName}` | Get a specific API subscription |
| `GET` | `/applications/{applicationId}/apisubscriptions/{apiSubscriptionName}/status` | Get subscription status |

#### Event Exposures

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/applications/{applicationId}/eventexposures` | List all event exposures for an application |
| `GET` | `/applications/{applicationId}/eventexposures/{eventExposureName}` | Get a specific event exposure |
| `GET` | `/applications/{applicationId}/eventexposures/{eventExposureName}/status` | Get event exposure status |
| `GET` | `/applications/{applicationId}/eventexposures/{eventExposureName}/eventsubscriptions` | List subscriptions to this exposure |

#### Event Subscriptions

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/applications/{applicationId}/eventsubscriptions` | List all event subscriptions for an application |
| `GET` | `/applications/{applicationId}/eventsubscriptions/{eventSubscriptionName}` | Get a specific event subscription |
| `GET` | `/applications/{applicationId}/eventsubscriptions/{eventSubscriptionName}/status` | Get event subscription status |

#### Event Types

EventType endpoints do not require an `applicationId` and are accessible to all authenticated users.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/eventtypes` | List all registered event types |
| `GET` | `/eventtypes/{eventTypeId}` | Get a specific event type by ID |
| `GET` | `/eventtypes/{eventTypeId}/status` | Get event type status |
| `GET` | `/eventtypes/{eventTypeName}/active` | Get the active version of an event type by name |

#### Deprecated Write Endpoints (HTTP 410 Gone)

All `POST`, `PUT`, and `DELETE` endpoints return `HTTP 410 Gone` with an [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) Problem Details response directing callers to the Rover API:

```json
{
  "type": "about:blank",
  "title": "Gone",
  "status": 410,
  "detail": "This endpoint is deprecated. Use the Rover API for write operations."
}
```

### Path Parameters

#### applicationId

A composite identifier in the format `{group}--{team}--{appName}`. The server resolves this to a Kubernetes namespace using the environment from the caller's JWT business context: `{env}--{group}--{team}`.

Example: For applicationId `eni--hyperion--echo-app` in the `playground` environment, the server queries namespace `playground--eni--hyperion`.

### Configuration

Configuration is loaded via [Viper](https://github.com/spf13/viper) with environment variable binding (using `_` as separator). For example, `SECURITY_ENABLED=true` maps to `security.enabled`.

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `ADDRESS` | `:8080` | Server listen address |
| `LOG_ENCODING` | `json` | Log output format (`json` or `console`) |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `SECURITY_ENABLED` | `true` | Enable JWT authentication |
| `SECURITY_TRUSTEDISSUERS` | `[]` | Comma-separated list of trusted JWT issuer URLs |
| `SECURITY_DEFAULTSCOPE` | `tardis:user:read` | Default scope when none is provided |
| `SECURITY_SCOPEPREFIX` | `tardis:` | Prefix for scope matching |
| `SECURITY_LMS_BASEPATH` | `""` | Base URL for external License Management Service |
| `DATABASE_FILEPATH` | `""` | BadgerDB storage path (empty = in-memory) |
| `DATABASE_REDUCEMEMORY` | `false` | Enable BadgerDB memory reduction mode |
| `INFORMER_DISABLECACHE` | `true` | Disable the Kubernetes informer cache |

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Regenerate OpenAPI types
make generate
```

### Project Structure

```
spy-server/
  api/                          # OpenAPI specifications
    stargate-openapi.yaml       # ApiExposure & ApiSubscription spec
    application-openapi.yaml    # Application spec
    event-openapi.yaml          # EventType, EventExposure & EventSubscription spec
    common.yaml                 # Shared schemas, parameters, security schemes
    uber-openapi.yaml           # Merged spec (generated)
    openapi-merge.json          # Merge configuration
  cmd/
    main.go                     # Entry point
  internal/
    api/                        # Generated Go types from OpenAPI (oapi-codegen)
    config/                     # Viper-based configuration
    controller/                 # Read-only controllers for each resource
    mapper/                     # CRD-to-API response mappers
      apiexposure/              # ApiExposure mapper
      apisubscription/          # ApiSubscription mapper
      application/              # Application mapper
      eventexposure/            # EventExposure mapper
      eventsubscription/        # EventSubscription mapper
      eventtype/                # EventType mapper
      status/                   # Status condition mapper
    pagination/                 # Offset/limit pagination utilities
    server/                     # Route registration and HTTP handlers
  pkg/
    log/                        # Structured logging (zap)
    store/                      # Kubernetes-backed in-memory stores (BadgerDB)
  tools/                        # Code generation tooling
  Makefile
```

## References

- [Control Plane Documentation](https://telekom.github.io/controlplane/)
- [Control Plane Overview](https://telekom.github.io/controlplane/docs/Overview/controlplane)
- [Components Overview](https://telekom.github.io/controlplane/docs/Overview/components)
- [Rover Server (write API)](../rover-server/) -- Use this for creating, updating, and deleting resources
- [Rover CLI](../rover-ctl/) -- CLI tool for interacting with the Rover API
- [common-server](../common-server/) -- Shared HTTP server framework (GoFiber, BadgerDB stores, security middleware)
- [common](../common/) -- Shared operator library (controller framework, handlers, config)

### API Groups (CRD domains)

- [`api.cp.ei.telekom.de`](../api/) -- Api, ApiExposure, ApiSubscription
- [`application.cp.ei.telekom.de`](../application/) -- Application
- [`admin.cp.ei.telekom.de`](../admin/) -- Environment, Zone
- [`approval.cp.ei.telekom.de`](../approval/) -- Approval, ApprovalRequest
- [`event.cp.ei.telekom.de`](../event/) -- EventExposure, EventSubscription, EventType
