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

## Optional capability components {#optional-capabilities}

Eventing and Permission are not included in the default overlay. Add either component to a [custom downstream overlay](#creating-a-custom-overlay) when needed:

```yaml
components:
  - https://github.com/telekom/controlplane//install/components/eventing/?ref=v0.22.0
  - https://github.com/telekom/controlplane//install/components/permission/?ref=v0.22.0
```

Use the same `install/components/<name>` pattern for other installation components. Each capability component is atomic: Eventing installs the Event and PubSub workloads and enables `FEATURE_PUBSUB_ENABLED`; Permission installs the Permission workload and enables `FEATURE_PERMISSION_ENABLED`. Do not add these flags to `global-config.env` yourself.

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

The Control Plane ships its deployment manifests as a set of [Kustomize](https://kustomize.io/) layers. Understanding this layout is helpful when you need to create a custom overlay — for example, to configure global settings, a component, or a storage backend.

```text
install/
├── base/                          # Shared foundation
├── bundle/                        # Core workloads without site configuration
│
├── overlays/
│   ├── default/                   # Ready-to-install production defaults
│   └── local/                     # Ready-to-install local environment
│
└── components/
    └── <name>/                    # Optional composable capability or service
```

The four layers work together as follows:

| Layer | Purpose |
|---|---|
| **Base** | Provides the shared namespace and trust infrastructure. |
| **Bundle** | Composes the core Control Plane workloads without site configuration or global defaults. |
| **Overlay** | Adds ready-to-install global defaults and environment choices. Use `install/overlays/default` for a direct production installation. |
| **Component** | Adds an optional capability and all global feature flags that capability requires. |

Each controller lives in its own directory in the repository (for example `secret-manager/`, `file-manager/`, `gateway/`) and carries a `config/default/` folder with its Kustomize manifests — including the deployment, RBAC rules, CRDs, network policies, and Prometheus metrics configuration.

#### Creating a custom overlay

To customise the deployment, create an overlay in your own repository that builds on `install/bundle`. Do not build a downstream overlay on `install/overlays/default`: capability components must be able to merge their flags into the `controlplane-env` generator owned by your overlay.

:::note
The bundle-based configuration workflow requires Control Plane v0.22.0 or newer.
:::

This is the pattern used by the repository's render verification fixture:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: controlplane-system

resources:
  - https://github.com/telekom/controlplane//install/bundle/?ref=v0.22.0

components:
  - https://github.com/telekom/controlplane//install/components/eventing/?ref=v0.22.0

configMapGenerator:
  - name: controlplane-env
    envs:
      - global-config.env
  - name: rover-env
    envs:
      - rover-config.env

patches:
  - target:
      kind: ConfigMap
      name: secret-manager-config
    path: secret-manager-config.yaml
```

`global-config.env` contains settings shared by workloads. `rover-config.env` contains only Rover overrides. Use unique final names such as `rover-env`, `admin-env`, or `rover-server-env`, because all operator ConfigMaps are created in the same namespace.

Configuration is applied from lowest to highest precedence:

```text
application defaults < controlplane-env < <component>-env < explicit container env
```

Applications own shipped defaults. Operators create a uniquely named `<component>-env` ConfigMap in their outer overlay when they need component-specific values; do not use `behavior: merge` or `behavior: replace` for a generator nested in the remote component. Explicit container environment entries are reserved for fixed runtime wiring and values sourced from Kubernetes Secrets.

Generated ConfigMaps retain Kustomize's content hash. A configuration change therefore updates the Pod template and triggers a rolling restart: global changes restart all consumers, while component changes restart that component. Applications read environment variables when they start, so new values take effect in the replacement Pods.

Never place passwords, tokens, keys, or other confidential values in these ConfigMaps. Store them in Kubernetes Secrets and reference those Secrets explicitly from the workload.

Structured configuration, such as `secret-manager-config`, remains separate from environment configuration. When the bundle is remote, patch the generated ConfigMap as shown above; this preserves hashing and reference rewriting. A local overlay may instead use `behavior: replace`, as `install/overlays/local` does.

Point your ArgoCD Application (or `kubectl apply -k`) at this custom overlay. The sections below show example structured configuration for Secret Manager and File Manager.

### Reference: configuring the Secret Manager {#reference-configuring-secret-manager}

The Secret Manager provides a secure API for storing and retrieving secrets on behalf of other controllers. It is deployed by the default overlay, but ships with **empty configuration** — you need to tell it which storage backend to use.

#### Backend options

| Backend | Best for | Description |
|---|---|---|
| **Kubernetes Secrets** | Development and simple setups | Stores secrets as native Kubernetes Secret resources. Easy to get started with, no external dependencies. |
| **Conjur** | Production environments | Stores secrets in [CyberArk Conjur](https://www.conjur.org/), which provides fine-grained access control and audit logging. |

#### Supplying the configuration

For a downstream overlay that consumes the remote bundle, patch the generated ConfigMap:

```yaml
patches:
  - target:
      kind: ConfigMap
      name: secret-manager-config
    path: secret-manager-config.yaml
```

Then create the patch file next to your overlay. The Secret Manager authenticates callers on an internal listener using in-cluster Kubernetes service account tokens. A minimal patch using the Kubernetes backend is:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: secret-manager-config
data:
  config.yaml: |-
    backend:
      type: kubernetes
```

With no `accessConfig`, any authenticated in-cluster service account is allowed. To restrict which service accounts may read or write secrets — recommended in production — add an `accessConfig` allow-list under the internal listener's `k8s` block:

```yaml
backend:
  type: kubernetes

listeners:
  internal:
    address: ":8443"
    k8s:
      audience: secret-manager
      accessConfig:
        - service_account_name: identity-controller-manager
          namespace: identity-system
          allowed_access:
            - secrets_read
        - service_account_name: organization-controller-manager
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
| **RustFS** | Self-hosted or on-premises setups | Stores files in a [RustFS](https://rustfs.com/) instance, which provides an S3-compatible API that you can run inside your cluster. |

#### Supplying the configuration

For a downstream overlay that consumes the remote bundle, patch the generated ConfigMap:

```yaml
patches:
  - target:
      kind: ConfigMap
      name: file-manager-config
    path: file-manager-config.yaml
```

Then create `file-manager-config.yaml` as a ConfigMap patch with the selected backend under `data.config.yaml`, following the same structure as the Secret Manager patch above. The examples below show the configuration file content for both backend options.

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
```

Replace the `bucket_name` and `role_arn` with your actual S3 bucket and IAM role. The `token_path` points to a projected service account token that the File Manager uses for STS authentication — this is configured automatically by the default deployment.

</details>

<details>
<summary>RustFS (self-hosted)</summary>

```yaml
backend:
  type: buckets
  endpoint: rustfs.rustfs.svc.cluster.local:9000
  bucket_name: controlplane-files
  access_key: myAccessKey
  secret_key: mySecretKey
  insecure_skip_tls: true
```

Replace the `endpoint`, `access_key`, and `secret_key` with your RustFS instance details. `insecure_skip_tls` selects HTTP and is suitable only when RustFS is intentionally exposed without TLS on a trusted network.

</details>

The local overlay deploys RustFS, creates the `controlplane-files` bucket, and configures File Manager automatically. No external object storage configuration is needed for `./hack/local-setup.sh`.

#### Security

The File Manager authenticates callers on its internal listener using in-cluster Kubernetes service account tokens. The controllers that interact with the File Manager (such as the API and Rover controllers) provide these automatically. To restrict which service accounts are allowed, add an `accessConfig` allow-list under the internal listener's `k8s` block; with no `accessConfig`, any authenticated in-cluster service account is allowed.

### Reference: eventing component details {#reference-eventing-component-details}

The eventing subsystem is an **optional feature** that enables event-driven communication between applications through publish/subscribe patterns. It is not included in the default overlay and must be explicitly enabled as described in [Optional capabilities](#optional-capabilities) above.

#### After enabling eventing

Deploying the eventing component installs the controllers and their CRDs, but does not activate eventing for any zone. To enable events in a specific zone, you need to create an `EventConfig` resource as part of your zone setup. This is covered in the [Environments & Zones](./environments-and-zones.md) guide.

For details on how application teams publish and consume events, see the user journey guides:

- [Exposing Events](../user-journey/exposing-events.mdx)
- [Subscribing to Events](../user-journey/subscribing-to-events.mdx)

## Internal certificate authority

Control Plane services in `controlplane-system` share one certificate authority (CA). This allows every internal client to validate every service certificate with the same trust bundle.

The default installation creates the CA in three steps:

```text
Issuer/controlplane-bootstrap-issuer (SelfSigned)
    |
    | creates and self-signs once
    v
Certificate/controlplane-root-ca
    |
    | stores its certificate and private key in Secret/controlplane-root-ca
    v
Issuer/controlplane-issuer (CA-backed)
    |
    | issues certificates for internal services
    v
Service certificates
```

### Bootstrap issuer

`Issuer/controlplane-bootstrap-issuer` solves the initial trust problem: a root CA must be self-signed because no higher CA exists to sign it. The bootstrap issuer is therefore used only to create `Certificate/controlplane-root-ca`. Internal service certificates must not reference it directly.

Using the self-signed issuer for each service certificate would create a separate trust anchor for every service. Clients would then need a separate trust bundle for File Manager, Secret Manager, Controlplane API, and every other internal service.

### Root CA

`Certificate/controlplane-root-ca` is marked as a CA certificate. cert-manager generates its private key, asks the bootstrap issuer to self-sign it, and stores both in `Secret/controlplane-root-ca`.

The private key remains in `controlplane-system` and is used only for certificate issuance. Client workloads receive the public CA certificate, not its private key.

### Service issuer

`Issuer/controlplane-issuer` is backed by `Secret/controlplane-root-ca`. It is the stable issuer referenced by all internal service `Certificate` resources:

```yaml
issuerRef:
  name: controlplane-issuer
  kind: Issuer
```

This separation keeps bootstrap and day-to-day issuance distinct. The bootstrap issuer creates the root CA once; the CA-backed service issuer uses that root to issue and renew service certificates throughout the installation's lifetime.

Internal clients trust `/var/run/secrets/trust-bundle/trust-bundle.pem`, distributed by trust-manager from `Bundle/controlplane-trust-bundle`. Because every service certificate chains back to `controlplane-root-ca`, one shared bundle is sufficient.

### Certificate profiles

The root CA and internal service certificates use explicit profiles for lifetimes, renewal windows, key algorithms, key usages, and private-key rotation. The deployed `Certificate` resources are the source of truth for these values.

Service identity is defined exclusively through DNS Subject Alternative Names. Leaf certificate subjects remain empty because `commonName` is not used for modern TLS hostname verification.

Changing the root CA profile on an existing installation is a CA rotation, not an ordinary certificate renewal. Publish both old and new CA certificates in the trust bundle before reissuing service certificates, and keep both trusted until all services and clients use the new chain.

## Use an externally managed issuer

Disable the default `controlplane-bootstrap-issuer`, `controlplane-root-ca` Certificate, and CA-backed `controlplane-issuer`. Provide a namespaced cert-manager `Issuer` named `controlplane-issuer` in `controlplane-system`, then replace the shared Bundle source with the public CA certificate for that issuer.

The replacement must preserve `kind: Issuer` and `name: controlplane-issuer`; service Certificate manifests do not need changes.

## Rotate the CA

Publish both old and new public CA certificates in `controlplane-trust-bundle`, reissue all service certificates from the new issuer, and remove the old CA only after every workload has loaded the new bundle and every serving certificate uses the new chain.

Removing the old CA requires rolling client workloads because the current refresher retains previously loaded roots until restart.

Never disable TLS verification during issuer replacement or rotation.
