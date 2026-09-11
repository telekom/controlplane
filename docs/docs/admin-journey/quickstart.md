---
sidebar_position: 1
---

# Quickstart

Get the Control Plane running on your laptop in about ten minutes. This guide sets up a local sandbox using [Kind](https://kind.sigs.k8s.io/) — a lightweight Kubernetes cluster that runs inside Docker — so you can explore the platform without touching a real environment.

:::warning Evaluation only
This setup is intended for **trying out** the Control Plane. It is not suitable for production use. For production deployments, see [Installation](./installation.md).
:::

## What you will get

By the end of this guide you will have:

- A Kind cluster with all Control Plane controllers running
- A sample environment with two zones
- A team with a dedicated namespace, identity client, and gateway consumer
- A sample API exposure and subscription

## Prerequisites

Make sure the following tools are installed on your machine:

| Tool | Purpose |
|------|---------|
| [Docker](https://docs.docker.com/get-docker/) | Container runtime for Kind |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | Kubernetes CLI |
| [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) | Local Kubernetes cluster |
| [ko](https://ko.build/install/) | Build Go container images |
| [Helm](https://helm.sh/docs/intro/install/) | Install dependencies |
| [Go](https://go.dev/doc/install) | Required by ko to build images |

## Step 0 — Clone the repository

This quickstart assumes you already cloned the Control Plane repository and are running commands from the repository root:

```bash
git clone https://github.com/telekom/controlplane.git
cd controlplane
```

## Step 1 — Run the setup script

Clone the repository and run the local setup script from the repository root:

```bash
./hack/local-setup.sh
```

This single command:

1. Creates a Kind cluster named `controlplane`
2. Installs cert-manager, trust-manager, and Prometheus Operator CRDs
3. Builds all controller images and loads them into the cluster
4. Deploys the full Control Plane

The script is idempotent — if the cluster already exists, it skips creation and moves on.

:::tip Selective rebuilds
After making code changes, you do not need to re-run the full setup. Use the `--build-only` flag to rebuild and redeploy:

```bash
# Rebuild all controllers
./hack/local-setup.sh --build-only

# Rebuild a single controller
./hack/local-setup.sh --build-only --only gateway

# Rebuild specific controllers
./hack/local-setup.sh --build-only --only gateway,rover
```
:::

## Step 2 — Apply sample resources

Once the controllers are running, apply the bundled sample resources to create an environment, a team, and an API exposure:

:::caution Update placeholder values before applying admin resources
The files under `install/overlays/local/resources/admin` (especially the zone definitions) contain placeholder values such as:

- Identity provider URLs and admin credentials
- Gateway URLs and admin client secrets
- Redis host and password

Before running `kubectl apply`, copy the zone example files and then replace placeholders in your local copies:

```bash
cp install/overlays/local/resources/admin/zones/dataplane1.example.yaml install/overlays/local/resources/admin/zones/dataplane1.yaml
cp install/overlays/local/resources/admin/zones/dataplane2.example.yaml install/overlays/local/resources/admin/zones/dataplane2.yaml
```

The copied `dataplane1.yaml` and `dataplane2.yaml` files are gitignored to help prevent accidental secret commits.

For details, see `install/overlays/local/README.md`.

For background on required zone infrastructure, see [Installation → Zone infrastructure](./installation.md#zone-infrastructure).
:::

```bash
# Create the environment and zones
kubectl apply -k install/overlays/local/resources/admin

# Create a group and team
kubectl apply -k install/overlays/local/resources/org

# Create a sample API exposure and subscription
kubectl apply -k install/overlays/local/resources/rover
```

## Step 3 — Verify

Check that all controllers are running:

```bash
kubectl get pods -n controlplane-system
```

All pods should show `Running` status. Then verify the sample resources were created:

```bash
# Environment
kubectl get environments -n controlplane

# Zones
kubectl get zones -n controlplane

# Team (and its auto-provisioned namespace)
kubectl get teams -n controlplane
kubectl get namespaces | grep controlplane--
```

You should see the namespace `controlplane--phoenix--firebirds` — this was automatically created when the team resource was applied.

## Explore the platform

With the platform running, here are a few things to try:

### Inspect the Rover resources

```bash
kubectl get rovers -n controlplane--phoenix--firebirds
kubectl get apispecifications -n controlplane--phoenix--firebirds
```

### Connect to the Secret Manager

```bash
kubectl port-forward -n controlplane-system svc/secret-manager 8443:8443
```

### Connect to the File Manager

```bash
kubectl port-forward -n controlplane-system svc/file-manager 8444:8443
```

## Clean up

To remove the local cluster entirely:

```bash
kind delete cluster --name controlplane
```

## Next steps

- [Installation](./installation.md) — Deploy the Control Plane on a production cluster
- [First Steps](./first-steps.md) — Bootstrap your first environment, zones, and teams
- [User Journey: Onboarding](../user-journey/onboarding.md) — Start using the platform as an application team
