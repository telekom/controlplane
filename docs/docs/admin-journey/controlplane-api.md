---
sidebar_position: 8
---

# ControlPlane API & Projector

The **ControlPlane API** and the **Projector** work together to give external clients — such as the Control Plane UI — read-only access to the platform's state. While the operators and custom resources remain the source of truth inside Kubernetes, these two components make that state available through a standard GraphQL API backed by a PostgreSQL database.

:::tip When do I need this?
If you only manage the Control Plane through `kubectl` and Rover-CTL, you do not strictly need these components. They become essential when you want to offer a **web-based UI** or any **external tooling** that queries the platform state without direct Kubernetes API access.
:::

## How They Work Together

The Projector and ControlPlane API follow a clear separation of concerns:

```mermaid
graph LR
    K8s["Kubernetes<br/><i>Custom Resources</i>"]
    P["Projector"]
    DB[("PostgreSQL")]
    API["ControlPlane API<br/><i>GraphQL</i>"]
    UI["UI / External Clients"]

    K8s -- watches --> P
    P -- writes --> DB
    API -- reads --> DB
    UI -- queries --> API

    classDef cluster fill:#e20074,stroke:#b8005c,color:#fff
    classDef service fill:#fff0f6,stroke:#e20074,color:#1a1a1a
    classDef db fill:#f0f0f0,stroke:#999,color:#1a1a1a

    class K8s cluster
    class P,API service
    class DB db
```

- The **Projector** runs inside the Kubernetes cluster as a read-only controller. It watches custom resources (teams, applications, API exposures, subscriptions, approvals) and continuously projects their current state into PostgreSQL. It never writes back to the cluster.
- The **ControlPlane API** is a read-only GraphQL server that queries the same PostgreSQL database. It provides paginated, filterable access to all projected resources with built-in team-level isolation.

Neither component modifies the Kubernetes state. All mutations continue to flow through Rover-CTL, Rover Server, or `kubectl`.

## Prerequisites

Before deploying the ControlPlane API and Projector, ensure you have:

- A running **PostgreSQL** instance (version 14 or later recommended)
- The Control Plane **operators** installed and running
- Network connectivity from the Projector to the Kubernetes API and to PostgreSQL
- Network connectivity from the ControlPlane API to PostgreSQL

Both components connect to the **same database**. The Projector creates and manages the schema automatically on startup.

## Configuring the Projector

The Projector is configured entirely through **environment variables**. The most important settings are:

### Database Connection

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `DATABASE_URL` | `postgres://localhost:5432/controlplane?sslmode=disable` | PostgreSQL connection string |
| `DB_MAX_OPEN_CONNS` | `100` | Maximum number of open database connections |
| `DB_MAX_IDLE_CONNS` | `30` | Maximum number of idle connections in the pool |
| `DB_CONN_MAX_LIFETIME` | `5m` | How long a connection can be reused before being closed |

### Reconciliation Tuning

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `MAX_CONCURRENT_RECONCILES` | `10` | How many resources are processed in parallel (per resource type) |
| `RECONCILE_TIMEOUT` | `7s` | Maximum time for processing a single resource |
| `PERIODIC_RESYNC` | `0s` | Interval for full re-synchronization (`0` = event-driven only) |
| `DEPENDENCY_DELAY` | `2s` | Wait time before retrying when a referenced resource is not yet available |

:::tip
In most deployments, the default values work well. You may want to increase `MAX_CONCURRENT_RECONCILES` on larger clusters with thousands of resources, or enable `PERIODIC_RESYNC` (e.g. `10m`) as a safety net to catch missed events.
:::

### Leader Election

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `LEADER_ELECTION` | `false` | Enable leader election for high-availability deployments |
| `LEADER_ELECTION_ID` | `projector.cp.ei.telekom.de` | Kubernetes resource name used for the leader election lease |

When leader election is enabled, you can run multiple Projector replicas for availability — only the elected leader will actively reconcile resources.

### Health and Metrics Endpoints

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `HEALTH_PROBE_BIND_ADDRESS` | `:8081` | Address for health (`/healthz`) and readiness (`/readyz`) probes |
| `METRICS_BIND_ADDRESS` | `:8090` | Address for Prometheus metrics |

## Configuring the ControlPlane API

The ControlPlane API is configured through a **YAML configuration file**, passed via the `--configfile` flag. Environment variable references (`$VAR_NAME`) are supported inside the YAML values.

### Minimal Configuration

```yaml
database:
  url: $DATABASE_URL              # e.g. postgres://user:pass@db-host:5432/controlplane?sslmode=require

server:
  address: ":8443"
  tls:
    enabled: true
    cert: /etc/tls/tls.crt
    key: /etc/tls/tls.key

security:
  enabled: true
  trustedIssuers:
    - https://your-idp.example.com/realms/master

graphql:
  playgroundEnabled: false        # set to true for development
```

### Configuration Reference

| Field | Default | Description |
| ----- | ------- | ----------- |
| `database.url` | `postgres://localhost:5432/controlplane?sslmode=disable` | PostgreSQL connection string |
| `server.address` | `:8443` | Listen address for the GraphQL server |
| `server.tls.enabled` | `true` | Whether to serve over HTTPS |
| `server.tls.cert` | `/etc/tls/tls.crt` | Path to the TLS certificate |
| `server.tls.key` | `/etc/tls/tls.key` | Path to the TLS private key |
| `security.enabled` | `false` | Enable JWT-based authentication |
| `security.trustedIssuers` | `[]` | List of trusted JWT issuer URLs |
| `graphql.playgroundEnabled` | `true` | Enable the interactive GraphQL Playground at `/graphql` |
| `log.level` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |

:::caution
In production, always set `security.enabled: true` and provide at least one trusted issuer. When security is disabled, the API grants admin-level access to all queries — this is intended for local development only.
:::

## Deployment

Both components are included in the standard Control Plane installation. They are deployed as Kubernetes Deployments and share the same PostgreSQL database, typically provided via a shared Secret (e.g. `controlplane-db`) containing the connection string.

### Verifying the Deployment

After installation, verify that both components are running:

```bash
kubectl get deployments -l app.kubernetes.io/part-of=controlplane | grep -E "projector|controlplane-api"
```

Check the Projector logs to confirm it is watching resources and syncing them into the database:

```bash
kubectl logs -l app=projector --tail=50
```

Check the ControlPlane API by accessing the GraphQL Playground (if enabled):

```bash
kubectl port-forward svc/controlplane-api 8443:8443
# Open https://localhost:8443/graphql in your browser
```

## Team Isolation

The ControlPlane API enforces **team-level data isolation** based on the caller's JWT token. When a client authenticates:

- **Team-scoped tokens** can only see resources belonging to their own team — applications, API exposures, subscriptions, and approvals.
- **Group-scoped tokens** can see resources belonging to all teams within their group.
- **Admin tokens** have unrestricted access to all resources.

Reference data such as **zones** and **groups** is visible to all authenticated users.

This means each team sees only its own slice of the platform — without any additional configuration from the administrator.

## Monitoring

The Projector exposes Prometheus metrics on its metrics endpoint (default `:8090`). Key metrics to watch:

| Metric | What It Tells You |
| ------ | ----------------- |
| `projector_reconcile_total` | Total reconciliations per resource type and outcome (success, error, skip) |
| `projector_reconcile_duration_seconds` | How long each reconciliation takes |
| `projector_db_operation_duration_seconds` | Database write latency per operation (upsert, delete) |
| `projector_idresolver_lookups_total` | Cache hit rate for foreign-key resolution — a low hit rate may indicate the cache is undersized |

:::tip
If you see frequent `dependency_missing` outcomes in the reconcile counter, it usually means the Projector is processing resources before their parent resources have been synced. This resolves automatically through retries, but you can reduce the noise by increasing `DEPENDENCY_DELAY`.
:::

## Troubleshooting

### Projector is not syncing resources

1. Check that the Projector has RBAC permissions to **read** the relevant custom resources (`teams`, `applications`, `apiexposures`, etc.)
2. Verify the database is reachable: the readiness probe at `/readyz` includes a database connectivity check
3. Look for `ErrDependencyMissing` in the logs — this indicates a resource's parent has not been synced yet and will be retried automatically

### ControlPlane API returns empty results

1. Confirm the Projector is running and has synced resources into the database
2. Check the caller's JWT token — team-scoped tokens will only see their own resources
3. If security is enabled, verify the token's issuer matches one of the `trustedIssuers` in the configuration

### GraphQL Playground is not accessible

The playground is served at `/graphql` and is enabled by default. In production configurations where `graphql.playgroundEnabled` is set to `false`, the endpoint returns the GraphQL API only (no UI).

## Next Steps

- [Architecture: ControlPlane API & Projector](../architecture/controlplane-api.mdx) — Understand the CQRS pattern, data model, and team isolation internals
- [Operations & Monitoring](./operations.md) — General observability guidance for the Control Plane
- [Components](../overview/components.md) — Overview of all platform components
