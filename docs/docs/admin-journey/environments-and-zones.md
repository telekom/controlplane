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

A Zone references the gateway and identity provider to use, along with optional Redis configuration and visibility settings:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
spec:
  visibility: World
  gateway:
    admin:
      url: https://gateway-admin.example.com
    presets:
      - name: default
        default: true
        urls:
          - hostname: api.dataplane1.example.com
            basePath: /
  identityProvider:
    url: https://idp.example.com
    admin:
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
Credential values (`clientSecret`, `password`, etc.) should not be committed to version control. See [Zone Secrets](#zone-secrets) below for how the Control Plane onboards these values automatically — in most cases you can leave them empty or use a secret reference instead of a clear-text value.
:::

### Gateway Admin Access

To configure routes at runtime, the Control Plane needs to authenticate against your gateway's **admin API**. The `gateway.admin` block holds this connection:

```yaml
gateway:
  admin:
    url: https://gateway-admin.example.com
    # clientSecret is optional — see below
```

You only need to provide the `url`. To handle authentication, the Control Plane automatically provisions a dedicated identity client (the `rover` client, described below) and generates its secret. If you want to set the client's secret yourself, provide `clientSecret`; otherwise it is generated for you.

#### The `rover` realm and client

On **every** zone reconciliation, the zone handler creates:

- An internal identity realm named **`rover`**, dedicated to platform-internal admin clients.
- A **`rover`** client inside that realm, used to authenticate against the gateway admin API. Its token issuer is `…/auth/realms/rover`.

This is not an opt-in feature and cannot be disabled — every zone gets its own `rover` realm and client.

:::note Previously a manual step
Earlier versions required administrators to create this realm and client by hand before a zone could work. This is now done for you automatically whenever the Zone is reconciled — no manual setup is needed.
:::

### Zone Secrets

A Zone references three sensitive values: the **identity provider admin password**, the **Redis password**, and the **gateway admin client secret**. You do not have to manage these as raw values in the Zone file.

When you apply a Zone, a defaulting webhook processes these fields:

| You provide | What happens |
|-------------|--------------|
| An empty value | A strong secret is generated for you. |
| The keyword `rotate` | A new secret is generated, replacing the previous one. |
| A clear-text value | The value is used as-is (and onboarded, see below). |
| A secret reference | Used directly — nothing is generated. |

If **Secret-Manager is enabled** in your installation, each generated or provided value is uploaded to Secret-Manager (under a `zones/<zone>/admin/...` path) and the Zone keeps only a reference. If it is **disabled**, the value is stored inline on the Zone instead.

:::tip
Re-applying a Zone never regenerates secrets by accident: when you omit a secret field on an update, the existing value is preserved. Use the `rotate` keyword when you explicitly want a fresh secret.
:::

### Zone Visibility

Zones can be configured with different visibility levels:

- **World** — The zone is accessible from outside the platform (public-facing APIs).
- **Enterprise** — The zone is accessible only within the organization's network.

### Managed Routes

Zones can optionally define **managed routes** — platform-managed gateway routes that are configured directly on the Zone resource rather than being created dynamically through Rover or API resources.

Each managed route has a **type** that determines its behavior:

| Type | Behavior |
|------|----------|
| **TeamAPI** | Authenticated route with token validation but no per-consumer ACLs. Used for team-facing platform APIs. |
| **Proxy** | Fully passthrough route that acts as a pure reverse proxy without any authentication or authorization. |

Example:

```yaml
spec:
  managedRoutes:
    routes:
      - name: team-api
        path: /team/api/v1
        url: https://my-team-api.internal.example.com/api/v1
        type: TeamAPI
      - name: health-proxy
        path: /health
        url: https://health-service.internal.example.com/
        type: Proxy
```

### Token Claims

The Control Plane automatically injects the following claims into all tokens issued for clients in a zone's identity realm:

| Claim | Type | Description |
|-------|------|-------------|
| `originZone` | Hardcoded | The name of the zone that issued the token. |
| `originStargate` | Hardcoded | The public gateway URL of the zone. |
| `clientId` | Session note | The OAuth2 client ID of the authenticated caller, populated automatically by Keycloak. |

These claims allow downstream services to identify the origin of a request without additional lookups.

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
  serverSendEventUrl: http://event-backend.dev.svc.cluster.local/sse
  publishEventUrl: http://event-backend.dev.svc.cluster.local/publish
  voyagerApiUrl: http://voyager.dev.svc.cluster.local
```

That's it. Identity clients, secrets, and mesh topology are configured automatically:

- **Admin client** — auto-created using the zone's internal identity realm.
- **Mesh client** — auto-created using the zone's identity realm.
- **Mesh topology** — defaults to full mesh (events distributed to all zones).
- **Client secrets** — auto-generated and managed by the controller.

### Mesh configuration options

By default, a full mesh topology is used. To restrict event distribution to specific zones, add an explicit `mesh` block:

```yaml
mesh:
  fullMesh: false
  zoneNames:
    - dataplane2
    - dataplane3
```

### Overriding identity client defaults

In most cases you do not need to specify identity clients. If your setup requires custom client IDs or specific realm references, you can provide them explicitly:

```yaml
admin:
  url: https://config-backend.example.com
  client:
    clientId: my-custom-admin-client
    realm:
      name: my-realm
      namespace: dev
mesh:
  fullMesh: true
  client:
    clientId: my-custom-mesh-client
    realm:
      name: my-realm
      namespace: dev
```

### Verifying readiness

After creation, the resource status is populated with generated references and URLs (for example `publishUrl`, `callbackUrl`, and `eventStore`).

If these fields appear and conditions are healthy, your zone is ready for event exposures and subscriptions.

## Next Steps

- [Organizations & Teams](./organizations-and-teams.md) — Set up teams within your environments
- [Architecture: Admin Domain](../architecture/admin.mdx) — Deep dive into the Admin domain
