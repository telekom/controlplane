// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

# Migration Operator

A Kubernetes operator that continuously synchronizes ApprovalRequest state from a legacy cluster to the current cluster.

## Overview

The Migration Operator watches `ApprovalRequest` resources and synchronizes their state with corresponding `Approval` resources in a legacy cluster using the Kubernetes API.

**Key Features:**
- ✅ Continuous state synchronization from legacy cluster
- ✅ Only updates existing resources (never creates new ones)
- ✅ Special `Suspended` → `Rejected` state mapping
- ✅ Efficient change detection to avoid unnecessary updates
- ✅ Production-ready with health checks, metrics, and leader election
- ✅ Uses native Kubernetes client (not REST API)

## Quick Start

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- Access to both legacy and new clusters
- Service account token from the legacy cluster

### 1. Create Service Account in Legacy Cluster

```bash
# In the LEGACY cluster
kubectl create serviceaccount migration-reader -n controlplane-system

kubectl create clusterrole migration-reader \
  --verb=get,list \
  --resource=approvals.approval.cp.ei.telekom.de

kubectl create clusterrolebinding migration-reader \
  --clusterrole=migration-reader \
  --serviceaccount=controlplane-system:migration-reader

# Get the token (save this for next step)
kubectl create token migration-reader -n controlplane-system --duration=8760h
```

### 2. Create Secret in New Cluster

```bash
# In the NEW cluster
kubectl create secret generic remote-cluster-token \
  --namespace=controlplane-system \
  --from-literal=server=https://legacy-cluster.example.com:6443 \
  --from-literal=token=<token-from-previous-step> \
  --from-file=ca.crt=/path/to/legacy-cluster-ca.crt
```

### 3. Deploy Operator

**Option A: Standalone Installation (includes CRDs)**

Use this if the approval operator is NOT installed:

```bash
# Deploy everything including CRDs
make deploy

# Or manually with kubectl
kubectl apply -k config/default
```

**Option B: Combined Installation (without CRDs)**

Use this if the approval operator IS already installed:

```bash
# Deploy without CRDs (CRDs come from approval module)
kubectl apply -k config/default-no-crds
```

**Note:** The operator requires the `ApprovalRequest` CRD to be installed. In standalone installations, it's included automatically. In combined installations with the approval operator, the CRD comes from the approval module.

### 4. Verify Deployment

```bash
# Check operator pod
kubectl get pods -n controlplane-system -l app.kubernetes.io/name=migration-operator

# Check logs
kubectl logs -n controlplane-system -l app.kubernetes.io/name=migration-operator -f
```

## How It Works

### Legacy API Compatibility

The operator connects to a **legacy cluster** using the old API group `acp.ei.telekom.de/v1` and converts resources to the new API group `approval.cp.ei.telekom.de/v1`.

**Key Conversions:**
- **API Group**: `acp.ei.telekom.de/v1` → `approval.cp.ei.telekom.de/v1`
- **State Casing**: `GRANTED` → `Granted`, `SUSPENDED` → `Suspended` (uppercase → PascalCase)
- **Strategy Casing**: `AUTO` → `Auto`, `SIMPLE` → `Simple` (uppercase → PascalCase)

### State Mapping

The operator maps legacy states to new cluster states:
- `PENDING` → `Pending`
- `GRANTED` → `Granted`
- `REJECTED` → `Rejected`
- `SEMIGRANTED` → `Semigranted`
- **`SUSPENDED` → `Suspended`** (then mapped to `Rejected` in the handler)

### Architecture

```
┌─────────────────┐      ┌──────────────────┐
│  Legacy Cluster │      │   New Cluster    │
│                 │      │                  │
│  ┌───────────┐  │      │  ┌─────────────┐ │
│  │ Approval  │  │ ───► │  │ Approval    │ │
│  │ (CRD)     │  │ K8s  │  │ Request     │ │
│  └───────────┘  │ API  │  └─────────────┘ │
│                 │      │        ▲         │
└─────────────────┘      │        │         │
                         │  ┌─────┴──────┐  │
                         │  │ Migration  │  │
                         │  │ Operator   │  │
                         │  └────────────┘  │
                         └──────────────────┘
```

### Reconciliation Flow

1. **Watch** ApprovalRequests in new cluster
2. **Compute** legacy Approval name from owner references
3. **Fetch** Approval from legacy cluster via Kubernetes API
4. **Compare** states using annotations
5. **Map** state (with Suspended → Rejected transformation)
6. **Update** ApprovalRequest if state changed
7. **Requeue** after 30 seconds

## Development

### Build

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run locally
make run
```

### Test

```bash
# Run all tests
make test

# Run with coverage
go test ./... -coverprofile=cover.out
go tool cover -html=cover.out
```

### Code Structure

```
migration/
├── cmd/
│   └── main.go                    # Entry point
├── internal/
│   ├── client/
│   │   └── remote_cluster_client.go  # K8s client for legacy cluster
│   ├── mapper/
│   │   └── approval_mapper.go     # State mapping logic
│   ├── handler/
│   │   └── approvalrequest/
│   │       └── handler.go         # Business logic
│   └── controller/
│       └── approvalrequest_migration_controller.go  # Controller
├── config/
│   ├── default/                   # Kustomize overlays
│   ├── manager/                   # Deployment
│   ├── rbac/                      # RBAC manifests
│   └── samples/                   # Sample configurations
├── Dockerfile
├── Makefile
└── go.mod
```

## Configuration

### Environment Variables

Set via command-line flags or deployment:

- `--metrics-bind-address` - Metrics endpoint (default: `:8080`)
- `--health-probe-bind-address` - Health probe endpoint (default: `:8081`)
- `--leader-elect` - Enable leader election (default: `false`)
- `--remote-cluster-secret-name` - Secret name (default: `remote-cluster-token`)
- `--remote-cluster-secret-namespace` - Secret namespace (default: `controlplane-system`)

### Remote Cluster Secret Format

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: remote-cluster-token
  namespace: controlplane-system
type: Opaque
stringData:
  server: https://legacy-cluster.example.com:6443
  token: <service-account-token>
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
```

## Troubleshooting

### Operator not starting

Check secret exists:
```bash
kubectl get secret remote-cluster-token -n controlplane-system
```

Check logs:
```bash
kubectl logs -n controlplane-system -l app=migration-operator
```

### State not syncing

Check if ApprovalRequest has owner references:
```bash
kubectl get approvalrequest <name> -o yaml | grep -A5 ownerReferences
```

Check legacy cluster connectivity:
```bash
kubectl exec -it <operator-pod> -n controlplane-system -- /bin/sh
# Try to access legacy cluster (if tools available)
```

### High CPU/Memory usage

Adjust resource limits in `config/manager/manager.yaml`:
```yaml
resources:
  limits:
    cpu: 1000m      # Increase
    memory: 1Gi     # Increase
  requests:
    cpu: 200m
    memory: 256Mi
```

## Uninstall

```bash
# Remove operator
make undeploy

# Remove secret
kubectl delete secret remote-cluster-token -n controlplane-system
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.

## License

Apache-2.0 - See [LICENSE](../LICENSE) for details.
