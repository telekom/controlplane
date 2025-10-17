// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

# Quick Start Guide

This guide will help you get the Migrator operator up and running in 5 minutes.

## Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- Access to both legacy and new clusters
- Go 1.23+ (for local development)

## Step 1: Build the Operator

```bash
cd migrator

# Ensure dependencies are up to date
go mod tidy

# Run tests
make test

# Build binary
make build

# Build Docker image
make docker-build IMG=<your-registry>/migrator:latest
make docker-push IMG=<your-registry>/migrator:latest
```

## Step 2: Create Remote Cluster Secret

On the **legacy cluster**, create a service account:

```bash
# Switch to legacy cluster
kubectl config use-context legacy-cluster

# Create service account
kubectl create serviceaccount migration-reader -n controlplane-system

# Create cluster role with read permissions
kubectl create clusterrole migration-reader \
  --verb=get,list,watch \
  --resource=approvals.acp.ei.telekom.de

# Bind role to service account
kubectl create clusterrolebinding migration-reader \
  --clusterrole=migration-reader \
  --serviceaccount=controlplane-system:migration-reader

# Generate token (valid for 1 year)
LEGACY_TOKEN=$(kubectl create token migration-reader -n controlplane-system --duration=8760h)

# Get server URL
LEGACY_SERVER=$(kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.server}')

# Get CA certificate
kubectl config view --raw --minify --flatten \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' \
  | base64 -d > /tmp/legacy-ca.crt

echo "Token: $LEGACY_TOKEN"
echo "Server: $LEGACY_SERVER"
echo "CA cert saved to: /tmp/legacy-ca.crt"
```

On the **new cluster**, create the secret:

```bash
# Switch to new cluster
kubectl config use-context new-cluster

# Create secret
kubectl create secret generic remote-cluster-token \
  --namespace=controlplane-system \
  --from-literal=server="${LEGACY_SERVER}" \
  --from-literal=token="${LEGACY_TOKEN}" \
  --from-file=ca.crt=/tmp/legacy-ca.crt

# Verify secret
kubectl get secret remote-cluster-token -n controlplane-system -o yaml
```

## Step 3: Deploy the Operator

```bash
# Update image in kustomization
cd config/manager
kustomize edit set image controller=<your-registry>/migrator:latest

# Deploy
cd ../..
kubectl apply -k config/default

# Or use make
make deploy IMG=<your-registry>/migrator:latest
```

## Step 4: Verify Deployment

```bash
# Check pod status
kubectl get pods -n controlplane-system -l domain=migration

# Check logs
kubectl logs -n controlplane-system -l domain=migration -f

# You should see:
# INFO  Registered migrators  count=1  migrators=["approvalrequest"]
# INFO  Setting up migrator  name=approvalrequest
# INFO  Successfully setup migrator  name=approvalrequest
# INFO  starting manager
```

## Step 5: Test Migration

Create a test ApprovalRequest in the new cluster:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: approval.cp.ei.telekom.de/v1
kind: ApprovalRequest
metadata:
  name: test-approval
  namespace: controlplane--eni--hyperion
  ownerReferences:
  - apiVersion: rover.cp.ei.telekom.de/v1
    kind: ApiSubscription
    name: consumer--api-name
    uid: "12345"
spec:
  state: Pending
  strategy: Simple
  approvalId: "test-approval-123"
  requesterId: "test-user"
EOF
```

Watch the operator logs:

```bash
kubectl logs -n controlplane-system -l domain=migration -f
```

You should see:
```
INFO  Reconciling resource for migration  migrator=approvalrequest  name=test-approval  namespace=controlplane--eni--hyperion
INFO  Computed legacy identifier  legacyNamespace=eni--hyperion  legacyName=apisubscription--api-name--consumer
INFO  Fetching from legacy cluster  namespace=eni--hyperion  name=apisubscription--api-name--consumer
...
```

## Step 6: Add More Migrators (Optional)

To add a new resource type:

1. Create migrator implementation:
   ```bash
   cp internal/migrators/rover/migrator.go.example internal/migrators/rover/migrator.go
   # Edit and implement the interface
   ```

2. Register in `cmd/main.go`:
   ```go
   import "github.com/telekom/controlplane/migrator/internal/migrators/rover"
   
   // In main():
   registry.Register(rover.NewRoverMigrator())
   ```

3. Rebuild and redeploy:
   ```bash
   make docker-build docker-push IMG=<your-registry>/migrator:latest
   kubectl rollout restart deployment migrator-controller-manager -n controlplane-system
   ```

## Troubleshooting

### Operator not starting

Check secret:
```bash
kubectl get secret remote-cluster-token -n controlplane-system
kubectl describe secret remote-cluster-token -n controlplane-system
```

Check logs:
```bash
kubectl logs -n controlplane-system -l domain=migration --tail=100
```

### Migration not happening

Check if ApprovalRequest has owner reference:
```bash
kubectl get approvalrequest <name> -n <namespace> -o jsonpath='{.metadata.ownerReferences}' | jq
```

Check operator logs for skip messages:
```bash
kubectl logs -n controlplane-system -l domain=migration | grep -i skip
```

### Certificate errors

Use the EKS API endpoint from the certificate:
```bash
# Get the error message showing valid hostnames
kubectl logs -n controlplane-system -l domain=migration | grep x509

# Update secret with the correct server URL
kubectl patch secret remote-cluster-token -n controlplane-system \
  --type='json' -p='[{"op": "replace", "path": "/data/server", "value":"'$(echo -n "https://correct-endpoint.eks.amazonaws.com" | base64)'"}]'
```

## Next Steps

- Read the [full README](README.md) for detailed documentation
- Check [examples](config/samples/) for sample configurations
- Add your own migrators following the plugin pattern

## Useful Commands

```bash
# View all resources
kubectl get all -n controlplane-system

# Check migrator metrics
kubectl port-forward -n controlplane-system svc/migrator-controller-manager-metrics-service 8080:8080
curl http://localhost:8080/metrics

# Watch ApprovalRequest changes
kubectl get approvalrequests -A -w

# Restart operator
kubectl rollout restart deployment migrator-controller-manager -n controlplane-system

# Uninstall
make undeploy
```
