---
sidebar_position: 2
---

# Local Development

This page covers how to set up a local development environment for working on the Control Plane.

## Prerequisites

Make sure the following tools are installed before you begin:

- **Go 1.25.8** — the primary language for all operators and services
- **Kubebuilder 4.9.0** — scaffolding and code generation for Kubernetes operators
- **A local Kubernetes cluster** — for example [kind](https://kind.sigs.k8s.io/) or [minikube](https://minikube.sigs.k8s.io/)
- **kubectl** — configured to communicate with your local cluster
- **Helm** — for installing dependencies such as cert-manager
- **pip + pre-commit** — for running pre-commit hooks (see [Contributing](./contributing.md))

## Repository Structure

The Control Plane is organised as a monorepo. Each domain lives in its own directory with a dedicated Go module (`go.mod`), `Makefile`, and — for operators — a Kubebuilder `PROJECT` file. Modules reference each other through Go `replace` directives.

### Operators

These are Kubebuilder-scaffolded controllers, each managing a specific domain:

`admin/` · `api/` · `application/` · `approval/` · `event/` · `gateway/` · `identity/` · `notification/` · `organization/` · `pubsub/` · `rover/`

### Services

Backend services that are not Kubernetes operators:

`common-server/` · `controlplane-api/` · `rover-server/` · `secret-manager/` · `file-manager/`

### CLI Tools

`rover-ctl/` — command-line interface for interacting with the Control Plane.

### Libraries

| Directory | Purpose |
|-----------|---------|
| `common/` | Shared operator framework (controller, handler, utility types) |
| `cpapi/`  | Shared API types used across services and operators |

### Installation

| Path | Purpose |
|------|---------|
| `install/base/` | Shared Kustomize base (namespace, issuer, controller references) |
| `install/overlays/default/` | Production overlay (pulls images from GitHub Container Registry) |
| `install/overlays/local/` | Local development overlay (images at `latest`, eventing enabled) |
| `install/components/eventing/` | Optional kustomize Component for event and pubsub controllers |
| `install.sh` | Interactive installer script for production/staging clusters |

### Tools

| Path | Purpose |
|------|---------|
| `tools/e2e-tester/` | End-to-end test runner |
| `tools/route-tester/` | Route testing utility |
| `tools/snapshotter/` | Cluster state snapshot tool |

### Code Generation

`hack/` — helper scripts including `local-setup.sh` (full local dev environment) and boilerplate templates for code generation.

## Local Installation

There are three ways to deploy the Control Plane to your local cluster.

### Option A — Full Local Setup (Recommended)

The `hack/local-setup.sh` script creates a Kind cluster, installs all prerequisites, builds every controller image with [ko](https://ko.build/), loads them into Kind, and deploys everything:

```bash
./hack/local-setup.sh
```

This is the fastest way to get a complete local environment. The script also supports incremental rebuilds:

```bash
# Rebuild all images and redeploy
./hack/local-setup.sh --build-only

# Rebuild a single controller
./hack/local-setup.sh --build-only --only gateway
```

### Option B — Interactive Installer

The `install.sh` script installs prerequisites (cert-manager, trust-manager, Prometheus CRDs) and downloads the kustomization file. You then apply it yourself:

```bash
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
kubectl apply -k .
```

### Option C — Manual Kustomize

If you prefer full control, apply the local overlay directly:

```bash
kubectl apply -k install/overlays/local
kubectl apply -k install/overlays/local/resources/admin
kubectl apply -k install/overlays/local/resources/org
kubectl apply -k install/overlays/local/resources/rover
```

## Working with a Domain

To develop on a single operator, navigate into its directory and use the provided Make targets:

```bash
cd <domain>

# Generate CRDs and code
make manifests
make generate

# Install CRDs into cluster
make install

# Run the operator locally (outside the cluster)
make run

# Build the binary
make build
```

Running the operator locally with `make run` starts the controller on your machine and connects it to the cluster configured in your current kubeconfig context. This gives you fast feedback without needing to build a container image.

## Running Tests

### Unit Tests

```bash
make test
```

Uses [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) with Kubernetes 1.31.0 assets. Output is formatted with [gotestfmt](https://github.com/gotesttools/gotestfmt).

### End-to-End Tests

```bash
make test-e2e
```

Runs [Ginkgo](https://onsi.github.io/ginkgo/) tests against a Kind cluster.

### Linting and Formatting

```bash
# Run the linter
make lint

# Run the linter and auto-fix issues
make lint-fix

# Format code
make fmt

# Run go vet
make vet
```

## Key Make Targets

Every operator directory exposes a consistent set of Make targets:

| Target | Description |
|--------|-------------|
| `manifests` | Generate CRDs, RBAC, and webhook manifests |
| `generate` | Generate DeepCopy methods |
| `fmt` | Run `go fmt` |
| `vet` | Run `go vet` |
| `test` | Run unit tests with envtest |
| `test-e2e` | Run end-to-end tests with Ginkgo |
| `lint` | Run golangci-lint |
| `build` | Build the manager binary |
| `run` | Run the operator locally |
| `install` | Install CRDs into the cluster |
| `uninstall` | Remove CRDs from the cluster |
| `deploy` | Deploy the operator to the cluster |
| `undeploy` | Remove the operator from the cluster |

## Tool Versions

The project pins the following tool versions to ensure reproducible builds:

| Tool | Version |
|------|---------|
| Kustomize | v5.4.3 |
| controller-gen | v0.20.1 |
| setup-envtest | release-0.19 |
| golangci-lint | v2.11.2 |
| envtest K8s version | 1.31.0 |

## Next Steps

- [Contributing](./contributing.md) — learn about the contribution workflow and code standards
- [Creating an Operator](./creating-an-operator.md) — scaffold a new operator from scratch
