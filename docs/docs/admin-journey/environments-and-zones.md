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
spec:
  realmName: dev
```

The `realmName` field defines the name used for identity and gateway realms in this environment. It is decoupled from the environment name — for example, you could name your environment `playground` but set `realmName: default` to keep existing realm URLs stable. If omitted, it defaults to the environment's `metadata.name`.

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

### Zone Readiness and Sub-Resource Events

A Zone becomes `Ready` only after its initial managed sub-resources are ready. After that first successful provisioning, readiness is latched: later child updates or failures do not make the Zone not ready, and changing the Zone spec does not reset the latch.

During initial provisioning, the Admin operator creates the identity provider and identity realms first. It waits until all three report `Ready=True` before creating the clients, gateway resources, and routes that depend on them. While waiting, the Zone reports `Ready=False` with reason `SubResourceNotReady` and names the resource that is not ready. Once initial provisioning is complete, this check no longer blocks later updates.

The Admin operator records a Warning event on the Zone for each managed sub-resource observed as not ready during reconciliation. Inspect these events to identify current child failures:

```bash
kubectl describe zone <name> -n <environment>
```

### Creating a Zone

A Zone references the gateways and identity provider to use, along with optional Redis configuration and visibility settings:

```yaml
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
spec:
  visibility: World
  gateways:
    - name: standard
      admin:
        identityProviderRef: primary
        url: https://gateway-admin.example.com
  identityProviders:
    - name: primary
      issuerHostname: idp.example.com
      allowedOrigins:
        - https://app.example.com
      admin:
        url: https://idp.example.com/auth/admin/realms
        clientId: admin-client
        userName: admin
        password: <your-idp-admin-password>
  presets:
    - name: default
      type: API
      default: true
      gatewayRef: standard
      identityProviderRef: primary
      urls:
        - hostname: api.dataplane1.example.com
          basePath: /
  redis:
    host: redis.example.com
    port: 6379
    password: <your-redis-password>
    enableTLS: true
```

`spec.identityProviders[].allowedOrigins` sets the Keycloak web origins for clients in the zone's default identity realm, whose name is recorded in `status.realmName`. Each entry must be a serialized origin (`scheme://host[:port]`) or `*`, where the host is a hostname, an IPv4 address, or a bracketed IPv6 address (for example `https://[2001:db8::1]:443`); paths, user information, queries, and fragments are not allowed. Any URL scheme is accepted. The `*` value allows all origins and should only be used when that access is intended. The internal rover realm is not affected. Updating this list updates existing clients; removing it clears their configured web origins.

### Traffic Types

Every entry in `spec.presets` declares which kind of traffic it routes through its required `type`:

| Type | Serves |
| ---- | ------ |
| `API` | Synchronous API traffic: exposures, subscriptions and proxy routes. |
| `AI` | Agentic traffic: MCP servers and agent cards. |
| `Event` | Asynchronous event traffic. |

A preset has exactly one type. A gateway has no type of its own — it serves the union of the
types of the presets that reference it. To send different traffic kinds to different gateway
instances, point their presets at different gateways:

```yaml
  # Excerpt: spec.gateways and spec.presets only. Preset urls and
  # identityProviderRef are omitted for brevity; both are required.
  gateways:
    - name: standard
      admin:
        identityProviderRef: primary
        url: https://gateway-admin.example.com
    - name: ai
      admin:
        identityProviderRef: primary
        url: https://ai-gateway-admin.example.com
  presets:
    - name: default
      type: API
      default: true
      gatewayRef: standard
    - name: events
      type: Event
      default: true
      gatewayRef: standard
    - name: ai
      type: AI
      default: true
      gatewayRef: ai
```

Here `standard` serves API and Event traffic because two presets of those types reference it,
while `ai` serves only AI traffic.

When the Control Plane needs a preset for a traffic kind — for example an AI Gateway route — it
selects among the presets of that type only. The default matching preset is preferred; if none is
marked as default, the first matching preset in the list is used.

Features configured on the Zone are inherited by every preset. A preset can override an inherited
feature by declaring the same name, including explicitly disabling it.

#### Rules the webhook enforces

A Zone is rejected at admission unless:

- **At least one `API` preset exists.** The API type carries the zone's representative profile
  and is used by every caller that has no traffic type of its own.
- **At most one preset per type is `default`.** Without one, list order determines the fallback.
- **Features may be configured on the Zone or any preset.** Preset values override inherited Zone
  values.
- **Every gateway is referenced by at least one preset.**

:::note
Because a gateway's types are derived from its presets, adding a preset of a new type to an
existing gateway is enough to make that gateway serve the new traffic kind — no change to the
gateway entry is needed.
:::

:::caution
Credential values (`clientSecret`, `password`, etc.) should not be committed to version control. See [Zone Secrets](#zone-secrets) below for how the Control Plane onboards these values automatically — in most cases you can leave them empty or use a secret reference instead of a clear-text value.
:::

### Gateway Admin Access

1. Set each gateway's admin API `url` and point `identityProviderRef` to the Zone's identity provider, as shown below.
2. Do not set gateway admin `clientId` or `clientSecret`; both are system-managed.
3. Apply the Zone and check its initial readiness and events as described in [Zone Readiness and Sub-Resource Events](#zone-readiness-and-sub-resource-events).

```yaml
gateways:
  - name: standard
    admin:
      identityProviderRef: primary
      url: https://gateway-admin.example.com
```

No manual realm or client setup is needed. All gateways in a Zone share one automatically provisioned `rover` client. For its lifecycle and secret handling, see [Admin Domain](../architecture/admin.mdx#identity-realms-and-the-rover-client).

### Private Provider CAs

Each Zone gateway and identity provider entry accepts an optional `tls` block naming a CA source ConfigMap in the
**Zone namespace**. Admin combines the certificates of **all** sources of the Zone into one Zone CA bundle, and every
gateway and identity provider of the Zone trusts that bundle in addition to the system roots, including entries
without a `tls` block of their own. The Zone is the trust boundary: a CA added for one provider is trusted by all
providers of the Zone, but never by another Zone. Name and key are
both required; only `data[key]` is read. Omitting `tls` (or its reference) means system trust. There is no inline
PEM, namespace field or insecure option.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: production # Zone namespace
  name: private-provider-ca
  labels:
    cp.ei.telekom.de/provider-ca: "true" # required
data:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    ...public CA certificate...
    -----END CERTIFICATE-----
---
# Zone spec excerpt
identityProviders:
  - name: primary
    tls:
      caBundleConfigMapRef:
        name: private-provider-ca
        key: ca.crt
gateways:
  - name: standard
    admin:
      identityProviderRef: primary
      url: https://gateway-admin.example.com
    tls:
      caBundleConfigMapRef:
        name: private-provider-ca
        key: ca.crt
```

Each source and the combined bundle must contain only PEM `CERTIFICATE` blocks (at most 256 KiB and 64 distinct
certificates; the limits apply to the union too). Once the Zone has any source, every provider endpoint must use
HTTPS: the IDP admin API, admin tokens, OIDC discovery, the Kong admin API and the gateway's OAuth issuer all use the
Zone bundle. Omitting all references means system trust for every provider.

When a token link needs OIDC discovery, Admin queries the issuer on each eligible reconciliation using the Zone
bundle. Presets sharing an issuer reuse that result only within the same reconciliation, not from a previous
one. An explicit preset `TokenUrl` takes precedence over the identity provider's `TokenUrl`; discovery is used only
when neither is set. A reconciliation that fails to prepare the identity provider publishes no new discovery metadata.

**Setup requirement:** add the `cp.ei.telekom.de/provider-ca: "true"` label **before** you set the reference, and keep
it during rotation and same-name recreation. Sources without the exact label are rejected even if the PEM is valid.
The label only puts the ConfigMap into the operators' caches; it is not an authorization boundary.

#### The Zone CA bundle

Admin does not hand the sources to the providers. It validates every source, merges their certificates into one
deduplicated bundle in a fixed order and writes it into the generated `<environment>--<zone>` namespace before any
provider references it. The bundle has a stable name per Zone, `provider-ca-<first 40 hex characters of the origin
hash>`, with the fixed key `ca.crt`, and carries:

| Metadata | Value |
| --- | --- |
| label `cp.ei.telekom.de/provider-ca` | `"true"` |
| labels `cp.ei.telekom.de/environment`, `cp.ei.telekom.de/zone`, `cp.ei.telekom.de/domain` | Environment, Zone name, `admin` |
| annotation `admin.cp.ei.telekom.de/zone-uid` | UID of the Zone |
| annotations `admin.cp.ei.telekom.de/provider-ca-origin-zone-namespace`, `-zone-name` | the origin tuple |
| annotation `admin.cp.ei.telekom.de/provider-ca-origin-hash` | full SHA-256 of the length-prefixed tuple `(zone UID, zone namespace, zone name)` |

The bundle names no single source. Copies of a single source written by earlier versions of this feature also carry
`-configmap` and `-key` annotations (part of their hash); they stay valid for references that still use them and are
not deleted. Bundles have no owner references or finalizers and are never deleted automatically. Admin only writes a
bundle whose existing metadata matches this origin; it never adopts a colliding object.

The Gateway and Identity operators enforce the same provenance whenever they resolve a reference: a
referenced ConfigMap whose name matches `provider-ca-<40 hex>` or which carries any of the origin annotations is
treated as an Admin-managed bundle or copy and needs the complete, hash-consistent origin, a matching Zone label, an
Environment label, no owner references and the key `ca.crt`. Partial or spoofed metadata blocks the provider
instead of falling back to system trust. Other labeled ConfigMaps referenced directly by manually authored
Gateways or IdentityProviders remain ordinary local sources and need only the label.

#### Trust caching in Gateway and Identity

Gateway and Identity read CA ConfigMaps through their informer cache, which only holds ConfigMaps labeled
`cp.ei.telekom.de/provider-ca: "true"`; a referenced ConfigMap without the label is reported as not found. They cache
each resolved CA reference for one minute and each failed resolution for five seconds. Status conditions and
provider connections use the same cache. Changes to a referenced ConfigMap, including a removed
label or deletion, do not trigger a reconciliation of Gateways, Routes, Consumers, IdentityProviders, Realms or
Clients. They take effect on the next reconciliation of each resource after its cached trust has expired: when the
resource or one of its dependencies changes, or at the periodic requeue (by default every 30 to 51 minutes). If a
refresh fails after the cached trust expired, reconciliation fails and retries; the operators never fall back to
older trust or system trust. A bundle rotation therefore reaches all providers of the Zone eventually, not within
a fixed time. The durable bundle Admin keeps is separate from this cache. Admin also reads sources and the bundle
through its label-filtered informer cache (without the one-minute CA cache), so it sees source changes once its
informer has observed them; an unlabeled source is reported as not found.

#### Status and failure behavior

The Zone condition `TrustSourcesReady` reports whether the requested sources are synchronized (not whether the
remote endpoint is reachable). Gateways, IdentityProviders and Realms report `TrustReady` for the trust they resolve.

| Situation | Behavior |
| --- | --- |
| Any requested source missing, invalid, forbidden or unlabeled, the combined bundle exceeds the limits, or a provider configuration is rejected | Admin writes no bundle and changes no provider in this reconciliation; `TrustSourcesReady=False` with reason `SourceInvalid` and the Zone is blocked. The existing bundle and published providers stay as they are. Once the source is repaired, the next reconciliation publishes the desired configuration. |
| Bundle cannot be written or does not validate | No provider is changed; `TrustSourcesReady=False` with reason `CopyInvalid`. |
| Invalid identity provider or gateway configuration | No provider is changed by the IdentityProvider failure; a failing gateway is not updated. The Zone is blocked. |
| Valid source update, added or removed reference | The bundle is replaced by the new union for every provider of the Zone; consumers pick up the new trust on their next reconciliation after their cached trust expires. |
| Last reference removed | All providers switch to system trust; the bundle is retained. |
| Bundle deleted while the Zone is live | Admin recreates it from valid sources. Consumers block while it is missing. |
| Bundle lost its `cp.ei.telekom.de/provider-ca: "true"` label | The bundle disappears from the operators' caches. Admin reports `TrustSourcesReady=False` with reason `CopyInvalid` and a message asking to restore the label, and retries; it does not overwrite or adopt the object. Restore the label manually (`kubectl label configmap <bundle> cp.ei.telekom.de/provider-ca=true -n <zone namespace>`); the next reconciliation recovers. |

#### Endpoint moves and Realm connections

Each Realm records the connection it currently uses in its status and uses it for cleanup. Changing the identity
provider admin URL or `issuerHostname` of a Zone is published like any other change: the IdentityProvider, the
dependent Gateway issuer and managed-route trusted issuers are updated, and Realms and Clients follow on their next
reconciliation. This is not a managed migration. Realms and Clients are created at the new endpoint; resources at the
old endpoint are left in place and must be removed manually. To move without leftovers, create a replacement
Zone/provider, move dependents, clean up the old resources and then retire them.

#### Rotation

Rotate by updating the source in place with old and new CAs together, switch the server certificates, then remove
the old CA. An accepted change of any source of a Zone changes trust for **every** provider and recorded Realm
connection of that Zone. A Zone change that Admin rejects (for example an invalid source) does not change the bundle.

#### Zone deletion and the retained bundle

Deleting a Zone keeps the generated namespace, providers, Realm connection status and the CA bundle so that later cleanup can
still connect. Admin stops synchronizing sources once deletion starts. Afterwards the bundle is authoritative; the
original sources may be deleted. To rotate trust after deletion, back up the bundle and replace `data.ca.crt` directly
with validated PEM (keep the label and all origin annotations unchanged); consumers pick up the change on their next
reconciliation after their cached trust expires. Remove remote business resources first, then providers, bundles and the namespace. Deleting a bundle early can
prevent remote cleanup.

#### Permissions

The Admin, Gateway and Identity operators only cache ConfigMaps carrying the provider-ca label, but they keep
clusterwide ConfigMap `get`, `list` and `watch` permission for all ConfigMaps; authoritative reads bypass the cache.
Admin may additionally `create`, `update` and `patch` (never `delete`) ConfigMaps for bundles. Gateway and Identity
use Leases for leader election and have no ConfigMap write permission. Restrict who may edit sources, bundles and
provider resources: they control provider trust.

### Zone Secrets

Configure the **identity provider admin password** and, if used, the **Redis password** on the Zone. Gateway admin credentials are managed automatically and are not part of this configuration.

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

### Zone Health

The Admin Operator creates one `<gateway>--zone-health` route for every gateway of every zone, including API, AI, and Event gateways. These routes are independent of managed routes and do not require DTC (`ConsumerFailover`).

The probe is exposed under each preset's base path on every hostname of presets referencing the gateway, without a `/spacegate` prefix. A `HEAD` request receives HTTP 200 directly from Kong; it does not check backend health or contact an upstream.

```bash
# For a preset with no base path:
curl -I https://<preset-hostname>/zone-health
# For a preset with base path /env1:
curl -I https://<preset-hostname>/env1/zone-health
# GET must not match the health route (another overlapping route may match):
curl -i https://<preset-hostname>/env1/zone-health
```

Admin sets `spec.traffic.healthProbe: true` on the Route. Gateway configures HEAD-only matching and attaches Kong's `request-termination` plugin with status 200. When false or omitted, normal routing applies, regardless of upstream path. The Route uses a placeholder upstream `http://localhost:8081/api/v1/zone-health` only because Kong requires a service. Apply the updated Route CRD and deploy the Gateway controller before the Admin controller; the Zone CRD is unchanged.

Hostnames and joined paths are sorted and deduplicated. Zone create/update admission rejects gateways whose combined presets exceed the Route limits of 20 distinct hostnames or 10 distinct joined paths, with an error on `spec.gateways[i].name`. These limits apply per gateway, not per preset or across the whole Zone. Base paths that normalize to the same joined path count once. The controller uses the same validation before creating identity or health routes, returning a blocked error for older invalid Zones rather than repeatedly failing CRD validation. Removing a gateway removes its health route, while removing managed routes does not.

### Managed Routes

Zones can optionally define **managed routes** — platform-managed gateway routes that are configured directly on the Zone resource rather than being created dynamically through Rover or API resources.

Each managed route has a **type** that determines its behavior:

The managed-route name `zone-health` is reserved for the platform health probe. Zone admission rejects it on create and update, regardless of route type or path. Name-collision protection relies on the validating webhook: existing Zones must not already use this name, and admission must not be bypassed.

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
Before creating an `EventConfig`, make sure the eventing subsystem is enabled in your Control Plane installation. Follow the steps in [Installation](./installation.md#optional-capabilities).
:::

After a zone is created, eventing is still not active for that zone by default. To enable the event feature, create an `EventConfig` resource for the zone.

`EventConfig` is the zone-level setup for the Event domain. It bootstraps the required event infrastructure in that zone (for example gateway routes, identity clients, and event store wiring).

### What `EventConfig` does

When an `EventConfig` is reconciled, the event controller prepares core building blocks used by event publishers and subscribers:

- **Identity clients** for event administration and cross-zone mesh communication
- **EventStore** connection used by the pub/sub runtime
- **Gateway routes and URLs** for publishing, callbacks, and (optionally) Voyager APIs

### Local and proxy zones

Every `EventConfig` describes one of two kinds of zone, and you must set **exactly one** of them:

- **Local zone** (`local:`) — a zone that runs its own event backend (Horizon). This is the common case.
- **Proxy zone** (`proxy:`) — a zone that runs *no* event backend of its own and uses a target zone's backend. See [Proxy zones](#proxy-zones) below.

:::caution Every zone that routes events needs an `Event`-typed preset
A zone that routes events must declare a preset with `type: Event` in `spec.presets`. The
admission webhook does **not** enforce this — a zone without one is accepted, and the failure
only appears when the `EventConfig` is reconciled.

In a full mesh this break is not local. Every meshed peer must have an `Event`-typed preset: a
single zone missing it blocks event reconciliation in *every* peer, not just in itself. Migrate
all meshed zones together; migrating them one at a time does not converge.
:::

### Creating an `EventConfig` for a local zone

Apply one `EventConfig` per zone. For a zone that runs its own event backend, put the backend connection details under a `local:` block:

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
  local:
    admin:
      url: https://config-backend.example.com
    serverSendEventUrl: http://event-backend.dev.svc.cluster.local/sse
    publishEventUrl: http://event-backend.dev.svc.cluster.local/publish
    voyagerApiUrl: http://voyager.dev.svc.cluster.local
```

The `local` block holds the connection to the configuration backend (`admin`) and the internal upstream URLs the gateway routes forward to:

| Field | Required | Purpose |
|-------|----------|---------|
| `admin.url` | Yes | Base URL of the event configuration backend. |
| `serverSendEventUrl` | Yes | Internal URL of the SSE backend, used as upstream for the SSE route. |
| `publishEventUrl` | Yes | Internal URL of the publish backend, used as upstream for the publish route. |
| `voyagerApiUrl` | No | Internal URL of the Voyager backend for event listing and redelivery. |

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

In most cases you do not need to specify identity clients. If your setup requires custom client IDs or specific realm references, you can provide them explicitly. The `admin` client lives inside the `local` block; the `mesh` client sits at the top level of the spec:

```yaml
spec:
  local:
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

### Proxy zones

Use a **proxy zone** when you want a zone to use an existing Horizon deployment in another zone rather than run its own.

To configure a proxy zone, set a `proxy:` block instead of `local:` and point it at the target zone:

```yaml
apiVersion: event.cp.ei.telekom.de/v1
kind: EventConfig
metadata:
  name: dataplane2-event-config
  namespace: dev
spec:
  zone:
    name: dataplane2
    namespace: dev
  proxy:
    targetZone:
      name: dataplane1
      namespace: dev
```

The target zone must be a **local** (non-proxy) zone running Horizon. Because a proxy zone has no backend of its own, it needs no `admin` or backend URLs — those all belong to the target zone.

When configuring Horizon's callback credentials, use the **target zone's EventConfig** client (`spec.mesh.client.clientId`, default `eventstore`) and its credentials, not the proxy zone's client if it differs.

:::warning Mutually exclusive

`local` and `proxy` are mutually exclusive. Every `EventConfig` must set exactly one of them, and the resource is rejected if both (or neither) are present.

:::

### Verifying readiness

After creation, the resource status is populated with generated references and URLs (for example `publishUrl`, `callbackUrl`, and `eventStore`).

If these fields appear and conditions are healthy, your zone is ready for event exposures and subscriptions.

## Next Steps

- [Organizations & Teams](./organizations-and-teams.md) — Set up teams within your environments
- [Architecture: Admin Domain](../architecture/admin.mdx) — Deep dive into the Admin domain
- [Architecture: Event Domain](../architecture/event.mdx) — Deep dive into the Event domain
