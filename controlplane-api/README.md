<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: CC0-1.0
-->

<p align="center">
  <h1 align="center">ControlPlane API</h1>
</p>

<p align="center">
  A read-only GraphQL API that serves as the query layer for the ControlPlane UI.
  It exposes teams, applications, API exposures/subscriptions, and approvals from a PostgreSQL database populated by a data sync operator.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#development">Development</a>
</p>

## About

The ControlPlane API provides a unified GraphQL interface over data synced from Kubernetes CRDs to PostgreSQL.
It is built with [ent](https://entgo.io/) (ORM + code generation), [gqlgen](https://gqlgen.com/) (GraphQL server), and [entgql](https://entgo.io/docs/graphql/) (bridge between the two).
The HTTP layer uses the [common-server](../common-server/) Fiber framework, consistent with other controlplane services.

Key characteristics:
- **Read-only** — all mutations are denied at the privacy layer
- **Relay-compliant** — cursor-based pagination, global node identification
- **Team-isolated** — viewers only see resources belonging to their teams (enforced by ent interceptors and privacy rules)
- **JWT-authenticated** — integrates with common-server's security middleware (JWT + BusinessContext)

## Architecture

```
HTTP Request (POST /graphql/query)
  │
  ├─ Fiber (common-server)         ← JWT + BusinessContext + CheckAccess
  ├─ adaptor.HTTPHandler           ← Fiber → net/http bridge
  ├─ gqlgen handler                ← GraphQL engine
  │   └─ ViewerFromBusinessContext ← JWT claims → Viewer (team/group/admin)
  ├─ ent client                    ← ORM with privacy + interceptors
  │   ├─ Privacy rules             ← require authenticated viewer
  │   └─ TeamFilterInterceptor     ← WHERE clauses based on viewer's teams
  └─ PostgreSQL (via pgx)
```

### Data Model

9 entities as ent schemas: **Team**, **Group**, **Environment**, **Zone**, **Application**, **ApiExposure**, **ApiSubscription**, **Approval**, **ApprovalRequest**.

Embedded types stored as JSON fields: Member, Upstream, ApprovalConfig, ApiInfo, RequesterInfo, DeciderInfo, Decision, AvailableTransition, ResourceStatus.

### Team Isolation

| Entity                        | Filter Logic                                                |
|-------------------------------|-------------------------------------------------------------|
| Team                          | `WHERE name IN (viewer.teams)`                              |
| Application                   | `WHERE owner_team.name IN (viewer.teams)`                   |
| ApiExposure / ApiSubscription | Via application → owner_team edge                           |
| Approval / ApprovalRequest    | Visible to both subscriber's team AND exposure owner's team |
| Group, Zone, Environment      | No filtering (public reference data)                        |

Admin viewers bypass all filtering.

## Development

### Prerequisites

- Go 1.25+
- PostgreSQL (for running the server; not needed for build/generate)

### Build

```bash
make generate   # Run ent + gqlgen code generation
make build      # Generate + compile
make test       # Run tests
make fmt        # Format code
make vet        # Run go vet
```

### Code Generation

The project uses a two-stage code generation pipeline:

```
ent/schema/*.go + schema.graphql + gqlgen.yml
       │                │               │
       ▼ (entc)         │               │
  ent.graphql           │               │
  ent/*.go              │               │
       │                │               │
       └────────────────┴───────────────┘
                        │
                        ▼ (gqlgen)
          internal/resolvers/*.generated.go
          internal/resolvers/*.resolvers.go
```

1. **ent** generates the ORM client (`ent/*.go`) and GraphQL schema (`ent.graphql`) from the ent schemas
2. **gqlgen** reads both `ent.graphql` and `schema.graphql` to generate the GraphQL runtime and resolver stubs

> [!WARNING]
> After running `make generate`, gqlgen may reshuffle resolver implementations between `*.resolvers.go` files and regenerate their import blocks.
> Always verify that custom imports (e.g., the `viewer` package) are still present after regeneration.

### Project Structure — Generated vs Manual Files

Most of the code under `ent/` is generated and should not be edited manually (marked with `// Code generated ... DO NOT EDIT.`).

| Path                                | Type                 | Description                                         |
|-------------------------------------|----------------------|-----------------------------------------------------|
| `ent/schema/`                       | **Manual**           | Entity definitions and mixins — the source of truth |
| `ent/entc/`                         | **Manual**           | Code generation configuration                       |
| `ent/*.go`, `ent/*/`                | Generated            | ORM client, builders, predicates, enums             |
| `ent.graphql`                       | Generated            | GraphQL schema derived from ent schemas             |
| `schema.graphql`                    | **Manual**           | Custom types and resolver extensions                |
| `gqlgen.yml`                        | **Manual**           | gqlgen configuration and model mappings             |
| `internal/resolvers/*.generated.go` | Generated            | GraphQL runtime wiring                              |
| `internal/resolvers/*.resolvers.go` | Generated (scaffold) | Resolver stubs — implementations are manual         |
| `internal/resolvers/model/`         | **Manual**           | Go types for custom GraphQL models                  |

### Adding a New Field

For a field managed by ent (stored in DB):
1. Edit the ent schema in `ent/schema/`
1. Run `make generate`
1. Done — ent + entgql handle the rest automatically

For a computed/custom field (not in DB):
1. Add the field to `schema.graphql` (e.g., via `extend type`)
1. Add the Go model to `internal/resolvers/model/` if needed
1. Map it in `gqlgen.yml`
1. Run `make generate`
1. Implement the resolver stub in the generated `*.resolvers.go` file

### Configuration

Configuration is loaded from a YAML file via `--configfile`. If omitted, built-in defaults are used.

```bash
controlplane-api --configfile /etc/controlplane/config/config.yaml
```

The following fields can be configured:

| Field                       | Default                                                                            | Description                              |
|-----------------------------|------------------------------------------------------------------------------------|------------------------------------------|
| `database.url`              | `postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable` | PostgreSQL connection string             |
| `server.address`            | `:8080`                                                                            | Server listen address                    |
| `graphql.playgroundEnabled` | `true`                                                                             | Enable GraphQL Playground at `/graphql/` |
| `security.enabled`          | `false`                                                                            | Enable JWT authentication                |
| `log.level`                 | `info`                                                                             | Log level (debug, info, warn, error)     |
