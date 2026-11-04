---
sidebar_position: 8
---

# ControlPlane API & Projector

The **ControlPlane API** and the **Projector** work together to give external clients — such as the Control Plane UI — read-only access to the platform's state. While the operators and custom resources remain the source of truth inside Kubernetes, these two components make that state available through a standard GraphQL API backed by a PostgreSQL database.

:::tip When do I need this?
If you only manage the Control Plane through `kubectl` and Rover-CTL, you do not strictly need these components. They become essential when you want to offer a **web-based UI** or any **external tooling** that queries the platform state without direct Kubernetes API access.
:::

## How They Work Together

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

    classDef cluster fill:#1a3d5c,stroke:#0f2b42,color:#fff
    classDef projector fill:#e20074,stroke:#b8005c,color:#fff
    classDef api fill:#2a7de1,stroke:#1a5fba,color:#fff
    classDef db fill:#f0f0f0,stroke:#999,color:#1a1a1a

    class K8s cluster
    class P projector
    class API api
    class DB db
```

- The **Projector** runs inside the Kubernetes cluster as a read-only controller. It watches custom resources (teams, applications, API exposures, subscriptions, approvals) and continuously projects their current state into PostgreSQL. It never writes back to the cluster.
- The **ControlPlane API** is a read-only GraphQL server that queries the same PostgreSQL database. It provides paginated, filterable access to all projected resources.

Neither component modifies the Kubernetes state. All mutations continue to flow through Rover-CTL, Rover Server, or `kubectl`.

## Prerequisites

Before enabling the ControlPlane API and Projector, ensure you have:

- A running **PostgreSQL** instance — in production use a managed database service; the `database` component provides one for local development (see below)
- The Control Plane **operators** installed and running (see [Installation](./installation.md))

Both components connect to the **same database** through the `controlplane-db` Secret. The Projector creates and manages the schema automatically on startup — no manual migration is needed.

## Enabling in the local overlay

The **local overlay** includes the ControlPlane API, Projector, and an in-cluster PostgreSQL instance out of the box. No additional steps are needed — just deploy using the local overlay as described in the [Quickstart](./quickstart.md):

```bash
kubectl apply -k install/overlays/local
```

The local overlay also:

- Includes the `database` component, which deploys a PostgreSQL pod with default credentials into the `controlplane-system` namespace
- Provides a pre-configured ControlPlane API config with the external listener's JWT auth set to `mock` and the GraphQL Playground enabled
- Sets all images to the `latest` tag with `IfNotPresent` pull policy

## Enabling in the default overlay

The default overlay ([`install/overlays/default/kustomization.yaml`](https://github.com/telekom/controlplane/blob/main/install/overlays/default/kustomization.yaml)) does **not** include the ControlPlane API and Projector by default — the relevant resource and image entries are present but commented out. The simplest way to enable them is to uncomment these lines directly in your copy of the overlay:

```yaml
resources:
  # ...existing controllers...
  # highlight-start
  # Uncomment to enable the controlplane-api and projector:
  - https://github.com/telekom/controlplane//controlplane-api/config/default/?timeout=120&ref=v0.18.0
  - https://github.com/telekom/controlplane//projector/config/default/?timeout=120&ref=v0.18.0
  # highlight-end

# highlight-start
# Uncomment to deploy an in-cluster PostgreSQL (development only):
components:
  - ../../components/database
# highlight-end

images:
  # ...existing images...
  # highlight-start
  - name: ghcr.io/telekom/controlplane/controlplane-api
    newTag: v0.18.0
  - name: ghcr.io/telekom/controlplane/projector
    newTag: v0.18.0
  # highlight-end
```

Alternatively, if you prefer to keep the upstream overlay untouched, create a custom overlay in your own repository that builds on the bundle — the same pattern used for [optional capabilities](./installation.md#optional-capabilities) and [other customisations](./installation.md#reference-kustomize-layout). This bundle-based workflow requires Control Plane v0.22.0 or newer.

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: controlplane-system

resources:
  - https://github.com/telekom/controlplane//install/bundle/?ref=v0.22.0
  - https://github.com/telekom/controlplane//controlplane-api/config/default/?timeout=120&ref=v0.22.0
  - https://github.com/telekom/controlplane//projector/config/default/?timeout=120&ref=v0.22.0

configMapGenerator:
  - name: controlplane-env
    literals:
      - FEATURE_PUBSUB_ENABLED=false
      - FEATURE_PERMISSION_ENABLED=false
      - FEATURE_AI_GATEWAY_ENABLED=false

images:
  - name: ghcr.io/telekom/controlplane/controlplane-api
    newTag: v0.22.0
  - name: ghcr.io/telekom/controlplane/projector
    newTag: v0.22.0
```

### Database

Both components expect a Kubernetes Secret named `controlplane-db` with a `url` key containing the PostgreSQL connection string. For production, create this Secret with your managed database credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: controlplane-db
  namespace: controlplane-system
stringData:
  url: postgres://user:password@your-db-host:5432/controlplane?sslmode=require
```

The database is the path of the URL (`/controlplane` above). With `DB_CREATE: "false"` (the default), the operations team must create the database in advance. Set `DB_CREATE: "true"` in the optional `projector-env` ConfigMap to let the Projector create it on startup, which needs the `CREATEDB` role attribute and an existing database named after the login user.

For development or testing, you can include the `database` component instead, which deploys an in-cluster PostgreSQL and creates the Secret automatically:

```yaml
components:
  - https://github.com/telekom/controlplane//install/components/database/?ref=v0.18.0
```

:::caution
The `database` component uses hardcoded credentials and is intended for development only. Always use a managed database service with secure credentials in production.
:::

### ControlPlane API configuration

The ControlPlane API reads its configuration from a ConfigMap named `controlplane-api-config`. When the custom overlay consumes the remote bundle, patch that generated ConfigMap:

```yaml
patches:
  - target:
      kind: ConfigMap
      name: controlplane-api-config
    path: controlplane-api-config.yaml
```

Then create `controlplane-api-config.yaml` next to your overlay as a ConfigMap patch, with the configuration under `data.config.yaml`. This preserves the generated name hash and rolling restart behavior.

#### Security modes

The API authenticates incoming requests on its external listener. The `listeners.external.jwt.mode` field controls how:

| Mode | JWT required | Signature validated | Use case |
|------|-------------|---------------------|----------|
| `jwt` | Yes | **Yes** | Production — full JWT validation against trusted issuers |
| `mock` | Yes | **No** | Local development / integration testing — JWT parsed but signature is not checked |

The default mode is `jwt` (secure by default). A deployment in `jwt` mode with no `trustedIssuers` will **panic at startup** — fail-closed by design.

#### Production configuration

```yaml
database:
  url: ${DATABASE_URL}

listeners:
  external:
    address: ":8443"
    jwt:
      mode: jwt
      trustedIssuers:
        - https://your-idp.example.com/realms/controlplane
```

The `DATABASE_URL` variable is injected from the `controlplane-db` Secret by the deployment manifest.

:::caution
`mode: jwt` requires at least one entry in `trustedIssuers`. The application will panic at startup if this is missing — fail-closed by design.
:::

#### Development / local overlay configuration

```yaml
database:
  url: ${DATABASE_URL}

listeners:
  external:
    address: ":8443"
    jwt:
      mode: mock

graphql:
  playgroundEnabled: true
```

:::warning
`mode: mock` accepts any JWT without validating its signature. Never use this in production.
:::

#### Testing configuration

```yaml
listeners:
  external:
    address: ":8443"
    jwt:
      mode: mock
      # No trustedIssuers needed — signatures are not validated
```

:::warning
`mode: mock` accepts JWTs without validating signatures. Only use for automated integration tests with controlled tokens.
:::

### Keycloak integration

To issue JWTs compatible with the ControlPlane API, configure Keycloak with the following protocol mappers:

#### Client setup

1. Create a **confidential client** in your Keycloak realm.
2. Enable **Service Accounts** for machine-to-machine authentication.
3. Add the protocol mappers below.

#### Required protocol mappers

| Mapper name | Mapper type | Token claim name | Example value |
|-------------|-------------|------------------|---------------|
| `env` | Hardcoded claim | `env` | `production` |
| `clientId` | User Session Note | `clientId` | `acme--platform--api-gateway` |
| `scope` | Hardcoded claim | `scope` | `tardis:admin:all` |

The `clientId` claim uses the format `<group>--<team>--<service>` and determines team-level access scoping. The `scope` claim controls the caller type (`tardis:team:all`, `tardis:group:all`, `tardis:admin:all`).

:::warning
When the `tardis:*` value (e.g. `tardis:admin:all`) is modeled as a Keycloak **client scope**, its **Include in token scope** setting must be enabled. If it is off, Keycloak omits the value from the `scope` claim and the caller is rejected or falls back to the default scope.
:::

Legacy `tardis:hub:*` scopes remain accepted at runtime as deprecated aliases for the corresponding `tardis:group:*` scopes.

#### Example JWT payload

```json
{
  "exp": 1719320400,
  "iat": 1719316800,
  "iss": "https://keycloak.example.com/realms/controlplane",
  "sub": "service-account-my-client",
  "env": "production",
  "clientId": "acme--platform--api-gateway",
  "scope": "tardis:admin:all"
}
```

## Running multiple environments in one cluster

By default, one ControlPlane API and one Projector serve every environment from a single database. To keep environments isolated, deploy **one Projector per environment**, each with its own database, and **one ControlPlane API** that serves all of them. Deployments without the settings below keep the single-environment behavior.

### How the isolation works

| Component | Setting | Effect |
|---|---|---|
| Projector | `ENVIRONMENT_FILTER=<environment>` | Watches only resources labeled `cp.ei.telekom.de/environment=<environment>`, and appends `-<environment>` to its leader election ID, so the instances do not compete for one lease. |
| Projector | `DATABASE_URL` | Writes into the environment's own database. With `DB_CREATE: "true"` it creates that database on startup (see [Database](#database)). |
| ControlPlane API | `kubernetes.environments` | Lists the environments it serves. Mutations write to the environment of the request. |
| ControlPlane API | `database.urlTemplate` | Replaces `database.url`. `<env>` is replaced by each environment, so every environment reads its Projector's database. |
| ControlPlane API | `graphql.perEnvironmentPathPrefix` | Serves each environment at `/<environment>/graphql/query` and its Playground at `/<environment>/graphql`. Requests whose environment does not match the path are rejected with `403`. |

### ControlPlane API

```yaml title="controlplane-api config.yaml"
graphql:
  perEnvironmentPathPrefix: true
kubernetes:
  environments:
    - dev
    - qa
```

Set the database URL through the environment, so the credentials stay in a Secret. Remove `DATABASE_URL`, because `database.url` and `database.urlTemplate` are mutually exclusive:

```yaml title="controlplane-api Deployment patch"
env:
  - name: DATABASE_URL
    $patch: delete
  - name: DB_USER
    valueFrom:
      secretKeyRef:
        name: controlplane-db
        key: username
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef:
        name: controlplane-db
        key: password
  - name: DATABASE_URLTEMPLATE
    value: postgres://$(DB_USER):$(DB_PASSWORD)@<db-host>:5432/<env>
```

The same settings are available as `KUBERNETES_ENVIRONMENTS=dev,qa` and `GRAPHQL_PERENVIRONMENTPATHPREFIX=true`. The API refuses to start if it lists several environments without both `perEnvironmentPathPrefix` and `urlTemplate`, or if `urlTemplate` does not contain `<env>`.

Keep in mind:

- **All environments run the same ControlPlane API version.** Upgrade the Projectors together, so every database has a schema this version can read.
- **The API opens one connection pool per environment.** Size `max_connections` of the database server accordingly.

### Projector per environment

Use one base and one small overlay per environment:

```text
environments/
├── kustomization.yaml   # resources: [dev, qa]
├── base/                # imports the Projector and sets the shared settings
├── dev/                 # name suffix and environment label
└── qa/
```

The base derives every environment-specific value from one label. Each overlay sets `app.kubernetes.io/instance` to its environment name, and the Projector reads it through the Kubernetes downward API:

```yaml title="environments/base/kustomization.yaml"
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/telekom/controlplane//projector/config/default/?ref=<version>

patches:
  - target:
      kind: Deployment
      name: projector-controller-manager
    patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: projector-controller-manager
      spec:
        template:
          spec:
            containers:
              - name: manager
                env:
                  - name: ENVIRONMENT_FILTER
                    value: $(CP_ENVIRONMENT)
                  - name: LEADER_ELECTION
                    value: "true"
  # The name is required but ignored, because target selects.
  - target:
      kind: Deployment
    patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: not-important
      spec:
        template:
          spec:
            containers:
              - name: manager
                env:
                  - name: CP_ENVIRONMENT
                    valueFrom:
                      fieldRef:
                        fieldPath: metadata.labels['app.kubernetes.io/instance']
                  - name: DB_USER
                    valueFrom:
                      secretKeyRef:
                        name: controlplane-db
                        key: username
                  - name: DB_PASSWORD
                    valueFrom:
                      secretKeyRef:
                        name: controlplane-db
                        key: password
                  - name: DATABASE_URL
                    value: postgres://$(DB_USER):$(DB_PASSWORD)@<db-host>:5432/$(CP_ENVIRONMENT)
                    valueFrom: null
```

Each overlay only sets the name suffix and the label:

```yaml title="environments/qa/kustomization.yaml"
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

nameSuffix: -qa

resources:
  - ../base

# Also the environment name: the base derives all environment settings from it.
labels:
  - includeSelectors: true
    pairs:
      app.kubernetes.io/instance: qa
```

Keep these rules in mind:

- **Import `config/default` per environment.** Each Projector then gets its own ServiceAccount, RBAC bindings, and metrics Service. Do not import the Projector once and add extra Deployments next to it: the shared metrics Service would select the pods of every environment.
- **Treat every environment the same way**, including the first. Use `nameSuffix` and the `instance` label with `includeSelectors: true` in every overlay, so each Service only selects its own pods.
- **Use the same environment names** in the overlays and in `kubernetes.environments`, so the API reads the database each Projector writes.
- **Check the order of environment variables.** Kubernetes only expands `$(VAR)` from variables defined earlier in the list. Strategic-merge patches can insert new entries before existing ones, which is why the shared patch above comes last. Inspect the rendered output with `kubectl kustomize`: if a reference stays unexpanded, all Projectors would use a database literally named `$(CP_ENVIRONMENT)`.

:::tip Separate database credentials
The example reuses one database role for all environments, which is fine for test environments. In production, give each environment its own database role.
:::

### Internal callers

Components that call the ControlPlane API inside the cluster, such as the organization server (`CPAPI_ENDPOINT`), use the environment's path, for example `https://controlplane-api.controlplane-system.svc.cluster.local:9443/qa/graphql/query`. They must send `X-Environment` with the same environment.

### Tokens and access

The token requirements from [Keycloak integration](#keycloak-integration) still apply. The `env` claim must name the environment in the path, so use one client per environment. The realms that the Control Plane creates for each zone, including the internal `rover` realm, do not add these claims (see [Token Claims](./environments-and-zones.md#token-claims)), so configure them in those realms as well.

## Verify the deployment

After installation, verify that both components are running:

```bash
kubectl get pods -n controlplane-system | grep -E "projector|controlplane-api"
```

Check the Projector logs to confirm it is watching resources and syncing them into the database:

```bash
kubectl logs -l app.kubernetes.io/name=projector -n controlplane-system --tail=50
```

If the GraphQL Playground is enabled, you can access it via port-forward:

```bash
kubectl port-forward svc/controlplane-api 8443:443 -n controlplane-system
# Open https://localhost:8443/graphql in your browser
```

When enabled, the Playground page at `/graphql` is publicly accessible. The Playground sends operations to `/graphql/query`, which always requires a JWT bearer token, including for introspection. In the Playground's **Headers** tab, add:

```json
{ "Authorization": "Bearer <your-token>" }
```

For local development with `mode: mock`, any JWT with valid claims works — the signature is not validated.

## Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| Projector pod not starting | Database not reachable | Check that the `controlplane-db` Secret exists and the connection string is correct. The readiness probe at `/readyz` includes a database check. |
| Projector logs show `ErrDependencyMissing` | Resources synced out of order | This is normal — the Projector retries automatically when a parent resource has not been synced yet. |
| ControlPlane API returns empty results | Projector not running, or JWT scoping | Confirm the Projector is syncing. Otherwise check that the caller's JWT has the expected team/group claims. |
| GraphQL Playground not accessible | Playground disabled in config | Set `graphql.playgroundEnabled: true` in the ControlPlane API configuration. |
| Playground queries return 401 | Missing or invalid Authorization header | Add `{ "Authorization": "Bearer <token>" }` in the Playground Headers tab. With `mode: mock`, any JWT with valid claims works. |
| 403 `Missing field 'env'` | The token has no `env` claim | Add a hardcoded `env` claim to the client's realm. See [Keycloak integration](#keycloak-integration). |
| 403 `No valid scope found` | The token's `scope` claim has no `tardis:<team\|group\|admin>:<read\|obfuscated\|all>` value | Add the client scope with **Include in token scope** enabled, and assign it as Default or request it in the token request. |
| A Projector instance projects nothing | `ENVIRONMENT_FILTER` does not match the resources' `cp.ei.telekom.de/environment` label | Check the label on the custom resources and the value of `ENVIRONMENT_FILTER`. Resources without the label are ignored when a filter is set. |

## Next steps

- [Architecture: ControlPlane API & Projector](../architecture/controlplane-api.mdx) — Understand the CQRS pattern, data model, and team isolation internals
- [Operations & Monitoring](./operations.md) — General observability guidance for the Control Plane
- [Components](../overview/components.md) — Overview of all platform components
