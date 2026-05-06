---
sidebar_position: 4
---

# Environments & Zones

Environments and zones form the foundational infrastructure of the Control Plane. They define *where* applications are deployed and *which* gateway and identity provider instances are used.

:::tip
For a visual overview of how environments, zones, and other resources fit together, see the [Resource Hierarchy](../overview/components.md#resource-hierarchy) diagram.
:::

## Environments

An Environment represents a logical separation of workloads — for example, `dev`, `staging`, or `production`. Each environment is a Kubernetes namespace, and all resources belonging to that environment (zones, groups, teams, applications) are created within it.

### Creating an Environment

To create an environment, apply an Environment resource. The namespace must match the environment name:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Environment
metadata:
  name: dev
  namespace: dev
```

:::tip
Create the Kubernetes namespace before applying the Environment resource, or use a namespace provisioning tool that handles this automatically.
:::

## Zones

A Zone represents a physical or logical deployment target within an environment. Each zone has its own gateway and identity provider configuration, and the Control Plane creates a dedicated namespace for the zone's resources.

Zones are the key building block for multi-cloud deployments. You can have zones pointing to different cloud providers (for example, one zone on AWS and another on Azure), and the Control Plane will manage API routing and event meshing across them.

### How a Zone is Set Up

Each zone is expected to have:

- **1 Gateway instance** — used for API routing and policy enforcement in that zone
- **1 Identity Provider (IDP) instance** — used for authentication and client management in that zone

By default, the current platform setup uses:

- **Kong** as the gateway, typically deployed with Helm via [`gateway-kong-charts`](https://github.com/telekom/gateway-kong-charts)
- **Keycloak** as the IDP, typically deployed with Helm via [`identity-iris-keycloak-charts`](https://github.com/telekom/identity-iris-keycloak-charts)

When creating the Zone resource in the Control Plane, you provide the connection details for exactly these zone-local instances (gateway + IDP). This keeps each zone self-contained and allows different zones to use separate runtime endpoints if needed.

### Creating a Zone

A Zone references the gateway and identity provider to use, along with Redis configuration and visibility settings:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
spec:
  visibility: World
  gateway:
    url: https://gateway.example.com
    circuitBreaker: true
    admin:
      url: https://gateway-admin.example.com
      clientSecret: <your-gateway-admin-secret>
  identityProvider:
    url: https://idp.example.com
    admin:
      url: https://idp-admin.example.com
      clientId: admin-client
      userName: admin
      password: <your-idp-admin-password>
  redis:
    host: redis.example.com
    port: 6379
    password: <your-redis-password>
    enableTLS: true
```

:::caution
Credential values (`clientSecret`, `password`, etc.) should not be committed to version control. In production, use a sealed-secrets solution or external secret management.
:::

### Zone Visibility

Zones can be configured with different visibility levels:

- **World** — The zone is accessible from outside the platform (public-facing APIs).
- **Enterprise** — The zone is accessible only within the organization's network.

### External Identifier Policies

Zones can enforce format and presence rules for business identifiers attached to Rovers (and their derived Applications). Each policy names a scheme, the regex its id must match, and whether the scheme is required in this zone.

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
spec:
  visibility: World
  # ... other fields ...
  externalIdPolicies:
    - scheme: psi
      required: true
      pattern: '^PSI-[0-9]{6}$'
    - scheme: icto
      required: false
      pattern: '^icto-[0-9]+$'
```

**Semantics**

- **Format is always enforced.** If a policy exists for a scheme and a Rover supplies a value under that scheme, the id must match `pattern`. Mismatches are rejected at admission time.
- **`required: true`** also enforces *presence*: a Rover in this zone must carry an identifier for the scheme. Rovers without it are rejected.
- **`required: false`** (default) leaves the identifier optional. If supplied, format is still checked; if omitted, the Rover is accepted.
- A zone with no `externalIdPolicies` does not validate any external identifier fields — users can supply any value (or none).

**Scheme → customer field mapping**

Internally, identifiers are stored as scheme-tagged entries. In customer-facing `rover.yaml` files they appear as scalar fields:

| Internal scheme | Customer scalar |
| --------------- | --------------- |
| `psi` | `psiid` |
| `icto` | `icto` |

**Operational notes**

- Policy changes are **not retroactive**. Admission webhooks only fire on create/update; existing Rovers remain valid until next edit. Plan rollouts accordingly — there is no sweep job to find existing violators.
- There is no observe-only / dry-run mode. Adding a policy immediately starts rejecting non-matching values on subsequent edits.

## Remote Organizations

:::caution Planned Feature
Remote Organizations are a **planned feature** and not yet fully supported. The API surface exists but may change, and end-to-end federation workflows are not production-ready. This section is provided for early awareness only.
:::

For cross-platform federation, the Control Plane supports Remote Organizations. A Remote Organization represents an external Control Plane instance that your platform can exchange API subscriptions and events with.

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: RemoteOrganization
metadata:
  name: partner-platform
  namespace: dev
spec:
  clientId: federation-client
  clientSecret: <your-federation-secret>
  issuerUrl: https://partner-controlplane.example.com/auth/realms/master
  zone:
    name: dataplane1
    namespace: dev
```

## Event Config

:::info
Before creating an `EventConfig`, make sure the eventing subsystem is enabled in your Control Plane installation. Follow the steps in [Installation](./installation.md#optional-enable-the-eventing-subsystem).
:::

After a zone is created, eventing is still not active for that zone by default. To enable the event feature, create an `EventConfig` resource for the zone.

`EventConfig` is the zone-level setup for the Event domain. It bootstraps the required event infrastructure in that zone (for example gateway routes, identity clients, and event store wiring).

### What `EventConfig` does

When an `EventConfig` is reconciled, the event controller prepares core building blocks used by event publishers and subscribers:

- **Identity clients** for event administration and cross-zone mesh communication
- **EventStore** connection used by the pub/sub runtime
- **Gateway routes and URLs** for publishing, callbacks, and (optionally) Voyager APIs

### Creating an `EventConfig`

Apply one `EventConfig` per zone:

```yaml
apiVersion: event.cp.ei.telekom.de/v1
kind: EventConfig
metadata:
  name: dataplane1-event-config
  namespace: dev
spec:
  zone:
    name: dataplane1
    namespace: dev
  admin:
    url: https://config-backend.example.com
    client:
      clientId: event-admin
      clientSecret: <your-event-admin-secret>
  serverSendEventUrl: http://event-backend.dev.svc.cluster.local/sse
  publishEventUrl: http://event-backend.dev.svc.cluster.local/publish
  voyagerApiUrl: http://voyager.dev.svc.cluster.local
  mesh:
    fullMesh: true
    client:
      clientId: event-mesh
      clientSecret: <your-event-mesh-secret>
```

### Mesh configuration options

- **`fullMesh: true`** — events can be distributed across all zones.
- **`fullMesh: false` + `zoneNames`** — events are only distributed to selected zones.

Example for partial mesh:

```yaml
mesh:
  fullMesh: false
  zoneNames:
    - dataplane2
    - dataplane3
  client:
    clientId: event-mesh
    clientSecret: <your-event-mesh-secret>
```

### Verifying readiness

After creation, the resource status is populated with generated references and URLs (for example `publishUrl`, `callbackUrl`, and `eventStore`).

If these fields appear and conditions are healthy, your zone is ready for event exposures and subscriptions.

:::caution
Do not commit secrets (for example `clientSecret`) to version control. Use your platform's secret management approach.
:::

## Next Steps

- [Organizations & Teams](./organizations-and-teams.md) — Set up teams within your environments
- [Architecture: Admin Domain](../architecture/admin.mdx) — Deep dive into the Admin domain
