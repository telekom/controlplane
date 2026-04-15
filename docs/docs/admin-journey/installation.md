---
sidebar_position: 2
---

# Installation

This guide covers how to deploy the Control Plane on a production Kubernetes cluster. The recommended approach is to use a GitOps tool such as [ArgoCD](https://argo-cd.readthedocs.io/) — the Control Plane ships as a set of Kustomize overlays that ArgoCD can sync directly from the GitHub repository.

:::tip Looking for a quick evaluation?
If you want to try the Control Plane on your laptop first, see the [Quickstart](./quickstart.md) guide instead.
:::

## What you will get

By the end of this guide you will have:

- The Control Plane controllers deployed and running in your cluster
- The required Custom Resource Definitions (CRDs) installed
- A repeatable GitOps setup (ArgoCD recommended) to manage upgrades
- A clear path for next steps: setting up environments, zones, groups, and teams

## Prerequisites

The Control Plane runs on Kubernetes and depends on a small number of cluster-level services. These should already be installed on your cluster before you deploy the Control Plane.

### Kubernetes cluster

- Kubernetes **v1.28 or later**
- Cluster-admin privileges for the initial deployment
- A GitOps tool such as ArgoCD (recommended) or Flux

### Required dependencies

The following components must be present in the cluster:

| Dependency | Minimum version | Purpose |
|---|---|---|
| [cert-manager](https://cert-manager.io/) | v1.18.2 | Issues TLS certificates for internal controller communication |
| [trust-manager](https://cert-manager.io/docs/trust/trust-manager/) | v0.19.0 | Distributes CA trust bundles across namespaces |
| [Prometheus Operator CRDs](https://prometheus-community.github.io/helm-charts/) | v23.0.0 | Enables metrics collection via ServiceMonitor resources |

Most organizations deploy these dependencies through ArgoCD as well. If you need reference manifests, see [Deploying dependencies with ArgoCD](#reference-deploying-dependencies-with-argocd) at the bottom of this page.

### Zone infrastructure

Each zone you create later will need its own instances of:

- **A Gateway** — [Kong](https://github.com/telekom/gateway-kong-charts) (deployed via Helm)
- **An Identity Provider** — [Keycloak](https://github.com/telekom/identity-iris-keycloak-charts) (deployed via Helm)
- **Redis** — for gateway rate-limiting and caching

These are not part of the Control Plane installation itself. You configure the connection details when you [create zones](./environments-and-zones.md) during the bootstrap phase.

For the overarching installation guidance for gateway and identity provider components, see the Open Telekom Integration Platform installation guides:

- [OTIP installation guides](https://github.com/telekom/Open-Telekom-Integration-Platform/tree/main/docs#installation-guides)

## Deploy with ArgoCD (recommended) {#deploy-with-argocd}

Create an ArgoCD Application that points at the Control Plane's default overlay. This deploys all controllers from a pinned release:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: controlplane
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/telekom/controlplane
    path: install/overlays/default
    targetRevision: v0.18.0   # pin to the release you want
  destination:
    server: https://kubernetes.default.svc
    namespace: controlplane-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - ServerSideApply=true
```

Apply this manifest to your cluster:

```bash
kubectl apply -f controlplane-app.yaml
```

ArgoCD will clone the repository at the specified tag, render the Kustomize overlay, and deploy all controllers into the `controlplane-system` namespace.

### Upgrading

To upgrade to a new release, update the `targetRevision` in your ArgoCD Application to the new tag (for example `v0.19.0`). ArgoCD will detect the change and sync the updated manifests.

## Optional: enable the eventing subsystem

The event and pubsub controllers are **not included** in the default overlay. These controllers enable event-driven communication between teams through publish/subscribe patterns.

To enable eventing, create a custom Kustomize overlay that includes the eventing component. Create a `kustomization.yaml` in your own repository:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/telekom/controlplane//install/overlays/default/?ref=v0.18.0

components:
  - https://github.com/telekom/controlplane//install/components/eventing/?ref=v0.18.0

images:
  - name: ghcr.io/telekom/controlplane/event
    newTag: v0.18.0
  - name: ghcr.io/telekom/controlplane/pubsub
    newTag: v0.18.0
```

Then point your ArgoCD Application at this custom overlay instead of the upstream path.

## Alternative: deploy with kubectl {#deploy-with-kubectl}

If you do not use a GitOps tool, you can apply the Control Plane directly with `kubectl`:

```bash
kubectl apply -k "https://github.com/telekom/controlplane/install/overlays/default?ref=v0.18.0"
```

## Verify the installation

After deployment, verify that all controllers are running:

```bash
kubectl get pods -n controlplane-system
```

All pods should reach the `Running` state within a few minutes. You can also check that the CRDs were installed:

```bash
kubectl get crds | grep cp.ei.telekom.de
```

### Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| Pods stuck in `Pending` | Missing CRDs or namespace | Ensure cert-manager and trust-manager are fully ready before deploying the Control Plane |
| Pods crash with `configmap not found` | trust-manager has not yet created the trust bundle | Wait for cert-manager Certificates to be issued, then trust-manager will create the ConfigMaps |
| CRD conflicts on apply | Existing CRD versions from a previous install | Use `ServerSideApply=true` in ArgoCD sync options or `kubectl apply --server-side` |

## Next steps

Once the platform is running, proceed to configure your first environment:

- [First Steps](./first-steps.md) — Create your first environment, zones, and teams
- [Environments & Zones](./environments-and-zones.md) — Deep dive on environment and zone configuration
- [Operations & Monitoring](./operations.md) — Day-2 operational tasks

---

## Appendix

### Reference: deploying dependencies with ArgoCD {#reference-deploying-dependencies-with-argocd}

If your cluster does not yet have the required dependencies, here are example ArgoCD Application manifests you can use as a starting point.

<details>
<summary>cert-manager</summary>

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://charts.jetstack.io
    chart: cert-manager
    targetRevision: v1.18.2
    helm:
      valuesObject:
        crds:
          enabled: true
  destination:
    server: https://kubernetes.default.svc
    namespace: cert-manager
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

</details>

<details>
<summary>trust-manager</summary>

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: trust-manager
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://charts.jetstack.io
    chart: trust-manager
    targetRevision: v0.19.0
    helm:
      valuesObject:
        app:
          trust:
            namespace: controlplane-system
  destination:
    server: https://kubernetes.default.svc
    namespace: cert-manager
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

:::note
The `app.trust.namespace` value must match the namespace where the Control Plane is deployed (`controlplane-system` by default).
:::

</details>

<details>
<summary>Prometheus Operator CRDs</summary>

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: prometheus-operator-crds
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://prometheus-community.github.io/helm-charts
    chart: prometheus-operator-crds
    targetRevision: v23.0.0
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

</details>

### Reference: Kustomize layout {#reference-kustomize-layout}

The Control Plane ships its deployment manifests as a set of [Kustomize](https://kustomize.io/) layers. Understanding this layout is helpful when you need to create a custom overlay — for example, to configure the Secret Manager backend, the File Manager storage, or to enable the optional eventing subsystem.

```text
install/
├── base/                          # Shared foundation
│   ├── kustomization.yaml         # Sets the namespace to controlplane-system
│   ├── namespace.yaml             # Namespace with required labels
│   └── issuer.yaml                # Self-signed cert-manager Issuer for internal TLS
│
├── overlays/
│   └── default/                   # Production overlay (recommended starting point)
│       └── kustomization.yaml     # Pulls all controller configs from GitHub at a pinned tag
│
└── components/
    └── eventing/                  # Optional component for event-driven features
        └── kustomization.yaml     # Adds the event and pubsub controllers
```

The three layers work together as follows:

| Layer | Purpose |
|---|---|
| **Base** | Creates the `controlplane-system` namespace with the labels that the Secret Manager and File Manager network policies require, and provisions a self-signed TLS issuer for internal controller communication. |
| **Overlay** | Composes the base with all individual controller configurations. The default overlay references each controller's config from GitHub at a pinned release tag. |
| **Component** | An optional add-on that can be composed into any overlay. The eventing component adds the event and pubsub controllers. |

Each controller lives in its own directory in the repository (for example `secret-manager/`, `file-manager/`, `gateway/`) and carries a `config/default/` folder with its Kustomize manifests — including the deployment, RBAC rules, CRDs, network policies, and Prometheus metrics configuration.

#### Creating a custom overlay

To customise the deployment — for instance, to supply your own Secret Manager or File Manager configuration — create a new overlay in your own repository that builds on top of the default overlay:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/telekom/controlplane//install/overlays/default/?ref=v0.18.0

# Add your patches, configMapGenerators, or components here
```

Point your ArgoCD Application (or `kubectl apply -k`) at this custom overlay instead of the upstream default. The sections below show what to add for the Secret Manager and File Manager.

### Reference: configuring the Secret Manager {#reference-configuring-secret-manager}

The Secret Manager provides a secure API for storing and retrieving secrets on behalf of other controllers. It is deployed by the default overlay, but ships with **empty configuration** — you need to tell it which storage backend to use.

#### Backend options

| Backend | Best for | Description |
|---|---|---|
| **Kubernetes Secrets** | Development and simple setups | Stores secrets as native Kubernetes Secret resources. Easy to get started with, no external dependencies. |
| **Conjur** | Production environments | Stores secrets in [CyberArk Conjur](https://www.conjur.org/), which provides fine-grained access control and audit logging. |

#### Supplying the configuration

In your [custom overlay](#reference-kustomize-layout), add a `configMapGenerator` that replaces the empty default with your own configuration file:

```yaml
configMapGenerator:
  - name: secret-manager-config
    behavior: replace
    files:
      - config.yaml=secret-manager-config.yaml
    options:
      disableNameSuffixHash: true
```

Then create a `secret-manager-config.yaml` next to your overlay. Here is an example that uses the Kubernetes backend:

```yaml
backend:
  type: kubernetes

security:
  enabled: true
```

When security is enabled, you can optionally restrict which service accounts are allowed to read or write secrets by adding an `access_config` section. This is useful in production to ensure that only the controllers that need secret access can reach the Secret Manager:

```yaml
security:
  enabled: true
  access_config:
    - service_account_name: identity-controller-manager
      deployment_name: identity-controller-manager
      namespace: identity-system
      allowed_access:
        - secrets_read
    - service_account_name: organization-controller-manager
      deployment_name: organization-controller-manager
      namespace: organization-system
      allowed_access:
        - onboarding_write
        - secrets_write
        - secrets_read
```

The available access rights are:

| Right | Grants access to |
|---|---|
| `secrets_read` | Reading secrets |
| `secrets_write` | Creating and deleting secrets |
| `onboarding_write` | Onboarding endpoints used during team provisioning |

### Reference: configuring the File Manager {#reference-configuring-file-manager}

The File Manager provides a storage API for files — primarily OpenAPI specifications. Like the Secret Manager, it is deployed by the default overlay with **empty configuration** and needs a storage backend.

#### Backend options

| Backend | Best for | Description |
|---|---|---|
| **Amazon S3** | Cloud-hosted production environments | Stores files in an S3 bucket. Authenticates using IAM role assumption via STS. |
| **MinIO** | Self-hosted or on-premises setups | Stores files in a [MinIO](https://min.io/) instance, which provides an S3-compatible API that you can run inside your cluster. |

#### Supplying the configuration

In your [custom overlay](#reference-kustomize-layout), add a `configMapGenerator` that replaces the empty default:

```yaml
configMapGenerator:
  - name: file-manager-config
    behavior: replace
    files:
      - config.yaml=file-manager-config.yaml
    options:
      disableNameSuffixHash: true
```

Then create a `file-manager-config.yaml`. The example below shows both backend options.

<details>
<summary>Amazon S3</summary>

```yaml
backend:
  type: buckets
  endpoint: s3.eu-central-1.amazonaws.com
  bucket_name: my-controlplane-files
  sts_endpoint: https://sts.amazonaws.com
  role_arn: arn:aws:iam::123456789012:role/my-file-manager-role
  token_path: /var/run/secrets/file-manager/file-manager-token

security:
  enabled: true
```

Replace the `bucket_name` and `role_arn` with your actual S3 bucket and IAM role. The `token_path` points to a projected service account token that the File Manager uses for STS authentication — this is configured automatically by the default deployment.

</details>

<details>
<summary>MinIO (self-hosted)</summary>

```yaml
backend:
  type: buckets
  endpoint: minio.minio.svc.cluster.local:9000
  bucket_name: controlplane-files
  access_key: myAccessKey
  secret_key: mySecretKey

security:
  enabled: true
```

Replace the `endpoint`, `access_key`, and `secret_key` with your MinIO instance details. The repository includes an example MinIO Helm values file under `file-manager/examples/minio/` that you can use as a starting point.

</details>

#### Security

When `security.enabled` is set to `true`, callers must provide a valid authentication token in the request header. The controllers that interact with the File Manager (such as the API and Rover controllers) handle this automatically.

### Reference: eventing component details {#reference-eventing-component-details}

The eventing subsystem is an **optional feature** that enables event-driven communication between applications through publish/subscribe patterns. It is not included in the default overlay and must be explicitly enabled as described in [Optional: enable the eventing subsystem](#optional-enable-the-eventing-subsystem) above.

#### After enabling eventing

Deploying the eventing component installs the controllers and their CRDs, but does not activate eventing for any zone. To enable events in a specific zone, you need to create an `EventConfig` resource as part of your zone setup. This is covered in the [Environments & Zones](./environments-and-zones.md) guide.

For details on how application teams publish and consume events, see the user journey guides:

- [Exposing Events](../user-journey/exposing-events.mdx)
- [Subscribing to Events](../user-journey/subscribing-to-events.mdx)
