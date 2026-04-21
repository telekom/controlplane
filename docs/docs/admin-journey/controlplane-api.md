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
- Provides a pre-configured ControlPlane API config with security disabled and the GraphQL Playground enabled
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

Alternatively, if you prefer to keep the upstream overlay untouched, create a custom overlay in your own repository that builds on top of it — the same pattern used for [eventing](./installation.md#optional-enable-the-eventing-subsystem) and [other customisations](./installation.md#reference-kustomize-layout):

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/telekom/controlplane//install/overlays/default/?ref=v0.18.0
  - https://github.com/telekom/controlplane//controlplane-api/config/default/?timeout=120&ref=v0.18.0
  - https://github.com/telekom/controlplane//projector/config/default/?timeout=120&ref=v0.18.0

images:
  - name: ghcr.io/telekom/controlplane/controlplane-api
    newTag: v0.18.0
  - name: ghcr.io/telekom/controlplane/projector
    newTag: v0.18.0
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

For development or testing, you can include the `database` component instead, which deploys an in-cluster PostgreSQL and creates the Secret automatically:

```yaml
components:
  - https://github.com/telekom/controlplane//install/components/database/?ref=v0.18.0
```

:::caution
The `database` component uses hardcoded credentials and is intended for development only. Always use a managed database service with secure credentials in production.
:::

### ControlPlane API configuration

The ControlPlane API reads its configuration from a ConfigMap named `controlplane-api-config`. In your custom overlay, provide a replacement config:

```yaml
configMapGenerator:
  - name: controlplane-api-config
    behavior: replace
    files:
      - config.yaml=controlplane-api-config.yaml
    options:
      disableNameSuffixHash: true
```

Then create a `controlplane-api-config.yaml` next to your overlay:

```yaml
database:
  url: ${DATABASE_URL}

security:
  enabled: true
  trustedIssuers:
    - https://your-idp.example.com/realms/master
```

The `DATABASE_URL` variable is injected from the `controlplane-db` Secret by the deployment manifest. The only setting you **must** configure for production is `security` — without it, the API grants admin-level access to all queries.

:::caution
In production, always set `security.enabled: true` and provide at least one trusted issuer. When security is disabled, the API grants unrestricted access — this is intended for local development only.
:::

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

## Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| Projector pod not starting | Database not reachable | Check that the `controlplane-db` Secret exists and the connection string is correct. The readiness probe at `/readyz` includes a database check. |
| Projector logs show `ErrDependencyMissing` | Resources synced out of order | This is normal — the Projector retries automatically when a parent resource has not been synced yet. |
| ControlPlane API returns empty results | Projector not running, or JWT scoping | Confirm the Projector is syncing. If security is enabled, check that the caller's JWT has the expected team/group claims. |
| GraphQL Playground not accessible | Playground disabled in config | Set `graphql.playgroundEnabled: true` in the ControlPlane API configuration. |

## Next steps

- [Architecture: ControlPlane API & Projector](../architecture/controlplane-api.mdx) — Understand the CQRS pattern, data model, and team isolation internals
- [Operations & Monitoring](./operations.md) — General observability guidance for the Control Plane
- [Components](../overview/components.md) — Overview of all platform components
