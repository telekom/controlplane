// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

# Migration Operator

A Kubernetes operator that continuously synchronizes ApprovalRequest state from a legacy cluster to the current cluster.

## Overview

The Migration Operator watches `ApprovalRequest` resources and synchronizes their state with corresponding `Approval` resources in a legacy cluster using the Kubernetes API.

**Key Features:**
- ✅ Continuous state synchronization from legacy cluster
- ✅ Automatic namespace transformation (`environment--group--team` → `group--team`)
- ✅ Smart approval name mapping with component swapping
- ✅ Only updates existing resources (never creates new ones)
- ✅ Special `Suspended` → `Rejected` state mapping
- ✅ **Special handling for `strategy: Auto`**: Sets corresponding **Approval** to `Rejected` if legacy has `Strategy=Auto` AND `State=Suspended`
- ✅ Efficient change detection to avoid unnecessary updates
- ✅ Detailed structured logging for debugging
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
2. **Compute** legacy Approval name from owner references (with component swapping)
3. **Transform** namespace from new to legacy format
4. **Fetch** Approval from legacy cluster via Kubernetes API
5. **Check** strategy:
   - If `Auto`: Apply special logic (see Auto Strategy Handling below)
   - Otherwise: Proceed with normal migration
6. **Compare** states using annotations
7. **Map** state (with Suspended → Rejected transformation)
8. **Update** ApprovalRequest if state changed
9. **Requeue** after 30 seconds

### Namespace Transformation

The operator automatically transforms namespaces between the two cluster formats:

**New Cluster:** `environment--groupName--teamName`  
Example: `controlplane--eni--hyperion`

**Legacy Cluster:** `groupName--teamName`  
Example: `eni--hyperion`

The environment prefix is stripped when querying the legacy cluster.

### Approval Name Transformation

The operator derives the legacy Approval name from the ApprovalRequest's owner reference and applies component swapping:

**Owner Reference Name (new cluster):** `rover-name--api-name`  
Example: `manual-tests-consumer-token-request--eni-manual-tests-echo-token-request-test-v1`

**Legacy Approval Name:** `apisubscription--api-name--rover-name`  
Example: `apisubscription--eni-manual-tests-echo-token-request-test-v1--manual-tests-consumer-token-request`

The components are swapped because the legacy naming convention places the API name before the consumer name.

### Auto Strategy Handling

The operator has **special logic** for ApprovalRequests with `strategy: Auto`:

**Background:**
- In the new cluster, ApprovalRequests with `Strategy=Auto` are automatically granted
- The system automatically creates a corresponding **Approval** resource with `State=Granted`
- The Approval name is **different** from the ApprovalRequest name
- The Approval name is stored in `ApprovalRequest.Status.Approval.Name`
- This Approval is what we need to update if the legacy cluster shows a problem

**Rule:** If the legacy Approval has **both** `Strategy=Auto` AND `State=Suspended`, the corresponding **Approval** (not ApprovalRequest) in the new cluster is set to `State=Rejected`.

**Why?** In the legacy cluster, Auto strategy approvals that are suspended indicate a policy violation or security issue. Since Auto approvals in the new cluster are automatically granted, we need to find and reject the auto-created Approval.

**Behavior:**
- ✅ **Legacy: `Strategy=Auto` + `State=Suspended`** → New: Find the Approval and set to `Rejected` + annotate Approval
- ✅ **Legacy: `Strategy=Auto` + `State=Granted`** → New: No migration, annotate ApprovalRequest with skip reason
- ✅ **Legacy: `Strategy=Auto` + other states** → New: No migration, annotate ApprovalRequest with skip reason
- ✅ **`Simple` or `FourEyes`** → Normal migration (full state sync on ApprovalRequest)

**Example log output for Auto+Suspended (Rejection):**
```
INFO  Handling Auto strategy ApprovalRequest  
  legacyStrategy=Auto 
  legacyState=Suspended 
  approvalRequestState=Granted

INFO  Legacy Approval is Auto+Suspended, looking for corresponding Approval to set to Rejected  
  approvalName=approval-a1b2c3d4e5  
  approvalNamespace=controlplane--eni--hyperion
  legacyApprovalName=apisubscription--api-name--rover-name

INFO  Setting Approval to Rejected  
  approvalName=approval-a1b2c3d4e5 
  oldState=Granted 
  newState=Rejected

INFO  Successfully set Auto strategy Approval to Rejected  
  approvalName=approval-a1b2c3d4e5
```

**Example log output for Auto+Granted (Skip with annotation):**
```
INFO  Handling Auto strategy ApprovalRequest
  legacyStrategy=Auto
  legacyState=Granted
  approvalRequestState=Granted

INFO  Legacy Approval is not Auto+Suspended, skipping migration for Auto strategy ApprovalRequest
  legacyStrategy=Auto
  legacyState=Granted

INFO  Annotated skipped Auto strategy ApprovalRequest
  skipReason="Auto strategy - legacy state is Granted (not Suspended)"
```

**Example resources for Auto+Suspended (Approval updated):**
```yaml
# ApprovalRequest (unchanged)
apiVersion: approval.cp.ei.telekom.de/v1
kind: ApprovalRequest
metadata:
  name: test-request
  namespace: controlplane--eni--hyperion
spec:
  strategy: Auto
  state: Granted  # Stays Granted
status:
  approval:
    name: approval-a1b2c3d4e5  # Reference to the Approval

---
# Approval (updated by migration)
apiVersion: approval.cp.ei.telekom.de/v1
kind: Approval
metadata:
  name: approval-a1b2c3d4e5  # Different from ApprovalRequest name!
  namespace: controlplane--eni--hyperion
  annotations:
    migration.cp.ei.telekom.de/last-migrated-state: "Rejected"
    migration.cp.ei.telekom.de/reason: "Auto strategy with Suspended state in legacy"
    migration.cp.ei.telekom.de/legacy-approval: "apisubscription--api-name--rover-name"
spec:
  state: Rejected  # Changed from Granted
```

**Example resources for Auto+Granted (ApprovalRequest annotated):**
```yaml
# ApprovalRequest (annotated to indicate skip)
apiVersion: approval.cp.ei.telekom.de/v1
kind: ApprovalRequest
metadata:
  name: test-request
  namespace: controlplane--eni--hyperion
  annotations:
    migration.cp.ei.telekom.de/skip-reason: "Auto strategy - legacy state is Granted (not Suspended)"
    migration.cp.ei.telekom.de/last-checked: "2025-01-20T16:30:00Z"
    migration.cp.ei.telekom.de/legacy-approval: "apisubscription--api-name--rover-name"
spec:
  strategy: Auto
  state: Granted  # No change
status:
  approval:
    name: approval-a1b2c3d4e5

---
# Approval (not updated by migration)
apiVersion: approval.cp.ei.telekom.de/v1
kind: Approval
metadata:
  name: approval-a1b2c3d4e5
  namespace: controlplane--eni--hyperion
spec:
  state: Granted  # Stays Granted
```

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

## Logging

The operator provides detailed structured logging for debugging:

```bash
# View logs
kubectl logs -n controlplane-system -l domain=migration -f
```

**Example log output:**
```
INFO  Reconciling ApprovalRequest for migration  name=test-approval  namespace=controlplane--eni--hyperion
INFO  Processing ApprovalRequest  state=Pending  hasOwnerRef=true
INFO  Computed legacy approval name  legacyApprovalName=apisubscription--api--rover
INFO  Computed legacy namespace  currentNamespace=controlplane--eni--hyperion  legacyNamespace=eni--hyperion
INFO  Fetching legacy approval from remote cluster  namespace=eni--hyperion  name=apisubscription--api--rover
INFO  Fetched legacy approval successfully  legacyState=Granted
INFO  Legacy state changed, migrating  oldState=Pending  newState=Granted
INFO  Successfully migrated state  newState=Granted
```

## Troubleshooting

### Operator not starting

Check secret exists:
```bash
kubectl get secret remote-cluster-token -n controlplane-system
```

Check logs for startup errors:
```bash
kubectl logs -n controlplane-system -l domain=migration
```

### State not syncing

**1. Check if ApprovalRequest has owner references:**
```bash
kubectl get approvalrequest <name> -n <namespace> -o jsonpath='{.metadata.ownerReferences}' | jq
```

If empty, migration will be skipped. The ApprovalRequest must have an owner reference to compute the legacy approval name.

**2. Check namespace format:**
```bash
kubectl get approvalrequest <name> -o jsonpath='{.metadata.namespace}'
# Should be: environment--group--team (e.g., controlplane--eni--hyperion)
```

**3. Watch logs during reconciliation:**
```bash
kubectl logs -n controlplane-system -l domain=migration -f
```

Look for messages like:
- `"Skipping migration for Auto strategy approval request"` - Expected for Auto strategy
- `"No owner reference found, skipping migration"` - Add owner reference
- `"No legacy Approval found in remote cluster"` - Check legacy cluster
- `"Legacy state unchanged, skipping update"` - Already migrated
- `"Failed to fetch legacy approval"` - Check connectivity/permissions

**4. Check legacy cluster connectivity:**
```bash
# Get computed legacy namespace and name from logs, then verify on legacy cluster
kubectl get approval <legacy-name> -n <legacy-namespace> --context=legacy-cluster
```

**5. Verify certificate and authentication:**
```bash
# Check if the secret has all required fields
kubectl get secret remote-cluster-token -n controlplane-system -o jsonpath='{.data}' | jq 'keys'
# Should show: ["ca.crt", "server", "token"]

# Decode and check server URL
kubectl get secret remote-cluster-token -n controlplane-system -o jsonpath='{.data.server}' | base64 -d
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

### Common Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| `failed to verify certificate: x509: certificate is valid for...` | Server URL doesn't match certificate | Use the EKS API endpoint from the certificate |
| `the server has asked for the client to provide credentials` | Invalid or expired token | Regenerate token on legacy cluster |
| `No owner reference found, skipping migration` | ApprovalRequest missing owner reference | Ensure ApprovalRequest is created with owner reference |
| `No legacy Approval found in remote cluster` | Approval doesn't exist on legacy cluster or wrong namespace/name | Check logs for computed namespace and name, verify on legacy cluster |

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
