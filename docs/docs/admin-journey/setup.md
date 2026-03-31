---
sidebar_position: 1
---

# Setup & Installation

This guide walks you through installing and configuring the Control Plane on a Kubernetes cluster.

## Prerequisites

Before you begin, make sure you have the following in place:

- A running Kubernetes cluster (v1.28 or later)
- `kubectl` configured to communicate with your cluster
- Cluster-admin privileges
- `helm` — for installing dependencies (cert-manager, trust-manager, Prometheus CRDs)
- `curl` and `jq` — if using the interactive installer script

## Installation

There are two ways to install the Control Plane: the interactive installer script (recommended) or a fully manual setup.

### Interactive Installer (Recommended)

The `install.sh` script installs prerequisites and downloads the kustomization file for the selected version:

```bash
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

The script:

- Optionally installs cert-manager, trust-manager, and Prometheus Operator CRDs via Helm
- Downloads the `kustomization.yaml` for the selected version
- Does **not** apply the kustomization — you apply it yourself or manage it with a GitOps tool

Once the script finishes, apply the manifests:

```bash
kubectl apply -k .
```

Or point an ArgoCD Application at the directory containing the downloaded `kustomization.yaml`.

### Manual Setup

If you prefer full control, install the prerequisites yourself and then download the kustomization file.

#### 1. Install cert-manager and trust-manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update

helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version v1.18.2 \
  --set crds.enabled=true

helm install trust-manager jetstack/trust-manager \
  --namespace cert-manager \
  --version v0.19.0
```

#### 2. Install Prometheus Operator CRDs

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus-operator-crds prometheus-community/prometheus-operator-crds \
  --version v23.0.0
```

#### 3. Download the kustomization file

Replace `<version>` with the desired release tag (for example, `v0.1.0`):

```bash
curl -LO "https://raw.githubusercontent.com/telekom/controlplane/<version>/install/overlays/default/kustomization.yaml"
```

#### 4. Apply

```bash
kubectl apply -k .
```

## Optional: Eventing Subsystem

The event and pubsub controllers are **not** installed by default. To enable them, uncomment the `components` section and the eventing image entries in `kustomization.yaml`, then re-apply.

## Verify the Installation

After installation, verify that all Control Plane components are running:

```bash
kubectl get pods -n controlplane-system
```

All pods should reach the `Running` state within a few minutes.

:::tip
Looking for a local development setup? See [Local Development](../developer-journey/local-development.md) in the Developer Journey.
:::

## Initial Configuration

The Control Plane is primarily configured through Kubernetes custom resources. Once the platform is running, the first steps are:

1. **Create an Environment** — Define at least one logical environment (for example, `dev`, `staging`, or `production`). See [Environments & Zones](./environments-and-zones.md).
2. **Create Zones** — Set up one or more deployment zones within the environment, each pointing to a gateway and identity provider instance. See [Environments & Zones](./environments-and-zones.md).
3. **Create Teams** — Organize your users into groups and teams so they can start using the platform. See [Organizations & Teams](./organizations-and-teams.md).

## Next Steps

- [Environments & Zones](./environments-and-zones.md) — Define where applications are deployed
- [Organizations & Teams](./organizations-and-teams.md) — Set up teams and access
- [Notification Templates](./notification-templates.md) — Configure notification delivery
