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

During OIDC discovery for a preset's token URL, the Admin operator also records a Warning event with reason `OIDCIssuerMismatch` on the Zone if the advertised issuer differs from the configured issuer or is missing. The event names the preset and both issuer values. A trailing slash alone does not count as a mismatch.

This warning does not block reconciliation: the configured issuer is retained and the discovered token endpoint is used if it passes validation. No issuer comparison is performed when an existing token URL is reused from the Zone status.

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

The Zone manages all attributes (`spec.attributes`) of its Realms (internal realms are excluded). Attributes set directly on these Realm resources are overwritten on the next reconciliation.

- **World zones:** the Realms get exactly one attribute, `frontendUrl`, set to `https://<issuerHostname>/auth`. The hostname is the identity provider's `issuerHostname`, or the host of its `admin.url` if no `issuerHostname` is set. If no hostname can be resolved, `frontendUrl` is set to an empty value, which means no frontend URL is configured.
- **Enterprise zones:** the Realms have no attributes.

See [Realm Attributes](../architecture/identity.mdx#realm-attributes).

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

### Custom CA Certificates

Use `tls.configMapRef` when the Control Plane must trust a private certificate authority (CA) for gateway or identity-provider connections. The certificates **extend the operator's system trust store**; they do not replace it. HTTPS still verifies certificate chains and hostnames. With no `tls` configuration, these connections use system roots only.

This setting covers gateway and identity-provider **admin APIs and authentication token endpoints**, plus the Admin operator's OpenID Connect (OIDC) discovery requests. It does not configure gateway-to-backend TLS, Redis TLS, or certificates served to API consumers. Existing internal CA mounts and Secret-Manager trust configuration are unchanged. There is no option to disable TLS verification in the operator resources.

Before using this feature, install the updated Zone, Gateway, IdentityProvider, and Realm CRDs and the corresponding operators and RBAC. Admin needs ConfigMap read/watch and write permissions; Gateway and Identity need `get`, `list`, and `watch`. ConfigMaps are read through the operators' Kubernetes caches, including ordinary ConfigMaps without platform labels.

#### Configure a Zone

Create each source ConfigMap in the **same namespace as the Zone**, not in its generated resource namespace. Both `name` and `key` are required; cross-namespace references and optional references are not supported. Store certificates as text under `data`, not `binaryData`. Source ConfigMaps need no Control Plane labels.

The following complete resource templates assume the `dev` Environment and namespace already exist. Replace each certificate placeholder with the base64 body of your actual PEM-encoded CA certificate, and supply your real endpoint and credential values before applying. Append additional complete `CERTIFICATE` blocks when an endpoint's CA differs from its token endpoint's CA. Never include a private key.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-ca
  namespace: dev
data:
  ca.pem: |
    -----BEGIN CERTIFICATE-----
    <GATEWAY_CA_CERTIFICATE_BASE64>
    -----END CERTIFICATE-----
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: identity-ca
  namespace: dev
data:
  ca.pem: |
    -----BEGIN CERTIFICATE-----
    <IDENTITY_CA_CERTIFICATE_BASE64>
    -----END CERTIFICATE-----
---
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dataplane1
  namespace: dev
  labels:
    cp.ei.telekom.de/environment: dev
spec:
  visibility: World
  gateways:
    - name: standard
      admin:
        identityProviderRef: primary
        url: https://gateway-admin.example.com
      tls:
        configMapRef:
          name: gateway-ca
          key: ca.pem
  identityProviders:
    - name: primary
      issuerHostname: idp.example.com
      admin:
        url: https://idp.example.com/auth/admin/realms
        clientId: admin-client
        userName: admin
        password: <your-idp-admin-password>
      tls:
        configMapRef:
          name: identity-ca
          key: ca.pem
  presets:
    - name: default
      type: API
      default: true
      gatewayRef: standard
      identityProviderRef: primary
      urls:
        - hostname: api.dataplane1.example.com
          basePath: /
```

Admin collects CA references from **every configured gateway and identity-provider entry**, not just the default preset. It validates them together and publishes their deduplicated union to one managed ConfigMap: `zone-ca-bundle`, key `ca-bundle.pem`, in `<environment>--<zone>` (`dev--dataplane1` here). Every generated Gateway and IdentityProvider uses that same local bundle, even if its own Zone entry has no `tls` field. Trust is therefore shared across the Zone, not isolated per endpoint. The current Zone provisioning model still requires exactly one identity provider; this feature does not add multi-IDP provisioning.

Admin's OIDC discovery uses the same validated union directly, without waiting to read back the managed ConfigMap. Existing discovery-result reuse is unchanged; updating a CA is not a new connectivity probe.

#### Validation and last valid configuration

Each referenced value must be nonempty and contain only parseable, header-free PEM `CERTIFICATE` blocks and whitespace. Certificates must be CAs with valid basic constraints and, when key usage is set, certificate-signing permission. Leaf certificates, private keys, malformed PEM, unmatched `BEGIN` lines, and unrelated content are rejected, even when a valid CA follows them; see [strict PEM parsing](../architecture/outbound-tls-clients.md#strict-pem-parsing). Duplicate certificates are removed and the bundle is put in a stable order. The resulting bundle must fit within the ConfigMap's 1 MiB data limit, including its key.

CA validity dates alone do not cause configuration rejection, which permits overlap during rotation. This does **not** disable validity-date checks during an HTTPS connection. Loading the system trust store must also succeed; the client does not silently switch to a custom-only trust store.

Admin resolves all sources before publishing the bundle or changing child connection settings. A missing ConfigMap, missing key, denied read, or invalid bundle blocks Zone reconciliation and leaves the **last valid published bundle and child connection settings intact**. The zone namespace may already have been created. Other Zone changes also wait until the CA configuration is repaired. Transient Kubernetes errors are retried.

“Last valid” means valid CA configuration, not a successful connection to every remote service. The bundle is published before child settings are updated, and these writes are not one transaction: a later failure does not roll back an already published bundle.

Do not edit the managed ConfigMap directly. Admin watches source ConfigMaps and queues referencing Zones when they change; it also watches the managed bundle to repair edits or deletion, provided all source inputs are valid. An unrelated ConfigMap already named `zone-ca-bundle` in the generated namespace is not overwritten. Resolve that naming collision without deleting trust material still in use.

#### Rotation and update timing

All CA reads use the manager's **cached Kubernetes client**, so updates are eventually consistent, not immediately visible to every operator. Each new client acquisition resolves and validates the current cached CA value before reusing a connection client. Once an invalid or missing value is observed, there is no fallback to a stale cached client. A corrupted or deleted managed bundle can therefore interrupt operations until Admin repairs it and caches catch up.

Source changes directly wake the Zone controller, but there is **no CA ConfigMap watch that immediately queues all affected Routes, Consumers, Realms, or Clients**. Those resources pick up changes on their next normal reconciliation or jittered retry. An already running operation can finish with its previously acquired client.

For a safe CA or endpoint migration:

1. Add the new CA while retaining the old CA in the source bundle.
2. Wait for the managed bundle to update and for affected resources to reconcile successfully.
3. Change the remote certificates or endpoint settings, keeping both CAs trusted while old and new settings may coexist.
4. Remove the old CA only after all affected operations, including pending deletions, no longer need it.

#### Removal, retention, and recovery

To return a Zone to system-only trust, remove **all** `tls` references from its gateway and identity-provider entries. Leaving even one reference keeps the Zone-wide union enabled. Do not empty or delete a referenced source as a removal mechanism: that is invalid configuration and preserves the last valid publication.

During reconciliation, Admin clears managed-bundle references from all retained Admin-managed Gateways and IdentityProviders for the Zone, including instances no longer listed in the Zone spec. It does not delete those children or change unmanaged resources. Cleanup runs after the other Zone provisioning steps, so earlier failures can delay it.

The managed bundle is retained while a Gateway or IdentityProvider in the generated namespace, or any Realm connection snapshot, still references it. Realm snapshots remember the CA reference and its originating namespace for later operations, including deletion. Admin retires the bundle on a later Zone reconciliation once those references have disappeared. Deleting the Zone itself retains the bundle alongside its children. Recreating the same-named Zone can reuse a retained bundle carrying the matching Admin/environment/Zone management labels; an old Zone UID does not prevent reuse.

For troubleshooting, inspect the Zone and the resource performing the remote operation:

```bash
kubectl describe zone dataplane1 -n dev
kubectl get configmap zone-ca-bundle -n dev--dataplane1 -o yaml
kubectl describe route <route-name> -n <route-namespace>
kubectl describe consumer <consumer-name> -n <consumer-namespace>
kubectl describe realm <realm-name> -n <realm-namespace>
kubectl describe client <client-name> -n <client-namespace>
```

Gateway and IdentityProvider readiness is not a proactive CA or connectivity check. Trust errors surface on Route/Consumer or Realm/Client operations. If a required remote deletion cannot acquire valid trust, its finalizer remains. Restore the referenced ConfigMap and key in the recorded namespace, correct its CA content or operator permissions, and allow reconciliation to retry. For a Zone-managed bundle, fix the sources and let Admin repair it while the Zone exists. Do not remove finalizers merely to bypass TLS failures: remote resources may be orphaned. The existing Client-deletion exception is unchanged: if its Realm is missing or not ready, remote client deletion is skipped.

#### Standalone Gateway and IdentityProvider

For resources created independently of Admin, put `tls.configMapRef` directly on `spec`. The source must be in the **Gateway's or IdentityProvider's own namespace**. The following templates share one source in `dev`; replace the certificate, endpoints, and credentials before applying. Include the CAs needed by both admin APIs and their token endpoints. If the resources are in different namespaces, create a source ConfigMap in each namespace.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: standalone-admin-ca
  namespace: dev
data:
  ca.pem: |
    -----BEGIN CERTIFICATE-----
    <ADMIN_AND_TOKEN_CA_CERTIFICATE_BASE64>
    -----END CERTIFICATE-----
---
apiVersion: gateway.cp.ei.telekom.de/v1
kind: Gateway
metadata:
  name: standalone-gateway
  namespace: dev
  labels:
    cp.ei.telekom.de/environment: dev
spec:
  admin:
    url: https://gateway-admin.example.com
    issuerUrl: https://idp.example.com/auth/realms/rover
    clientId: gateway-admin
    clientSecret: <your-gateway-admin-client-secret>
  tls:
    configMapRef:
      name: standalone-admin-ca
      key: ca.pem
---
apiVersion: identity.cp.ei.telekom.de/v1
kind: IdentityProvider
metadata:
  name: standalone-idp
  namespace: dev
  labels:
    cp.ei.telekom.de/environment: dev
spec:
  adminUrl: https://idp.example.com/auth/admin/realms
  adminClientId: admin-client
  adminUserName: admin
  adminPassword: <your-idp-admin-password>
  tls:
    configMapRef:
      name: standalone-admin-ca
      key: ca.pem
```

Standalone resources use the same CA validation but have **no managed copy or durable last-valid fallback**. An invalid, missing, or unreadable source blocks the next affected operation once observed by the cache, even if a client was previously cached. Fix the source and wait for the operation's next reconciliation. Omit `spec.tls` for system-only trust; allow Realm connection snapshots to update before removing an old IDP source needed for deletion. Standalone credentials and backing admin accounts must already be configured; Admin does not provision them for these examples.

For implementation details, see [Outbound TLS Clients](../architecture/outbound-tls-clients.md).

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
