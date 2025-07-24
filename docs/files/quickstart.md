<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Quickstart Guide

This guide helps you quickly set up the Open Telekom Integration Platform Control Plane in a local development environment. Perfect for testing, development, and learning how the Control Plane works without the complexity of a full production deployment.

> **Production Setup:** For production Kubernetes environments, see the comprehensive [Installation Guide](./installation.md).
> 
> **Architecture:** To understand the overall system design, review the [Control Plane Architecture](../../README.md#architecture) and [architecture diagram](../img/CP_Architecture.drawio.svg).

## Quick Setup (Local Development)

### Prerequisites

**Minimal Requirements:**
- **Kubernetes cluster**: Local cluster (kind, minikube, k3s, or Docker Desktop)
- **kubectl**: Kubernetes command-line tool
- **Internet connection**: For downloading container images

**Verify your setup:**
```bash
# Check kubectl is working
kubectl version --client

# Check cluster access
kubectl cluster-info

# Ensure you have admin permissions
kubectl auth can-i create clusterroles --all-namespaces
```

### Fast Installation (5 minutes)

**Step 1: Install Control Plane Operators**

```bash
# Clone or download the installation files
curl -sSL https://raw.githubusercontent.com/telekom/controlplane/main/install/local/kustomization.yaml -o kustomization.yaml

# Install all operators and CRDs
kubectl apply -k https://github.com/telekom/controlplane/install/local?ref=main
```

**Step 2: Verify Installation**

```bash
# Check that all operators are running (this may take 2-3 minutes)
kubectl get pods -A -l control-plane=controller-manager

# Wait for all pods to be ready
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -A --timeout=300s
```

**Step 3: Set Up Sample Environment**

```bash
# Install admin resources (environment and zone)
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/admin?ref=main

# Verify environment and zone are ready
kubectl wait --for=condition=Ready -n default zones/zone-a --timeout=60s
```

**Step 4: Create Organization Structure**

```bash
# Install organizational resources (group and team)
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/org?ref=main

# Verify team is ready
kubectl wait --for=condition=Ready -n default teams/group-sample--team-sample --timeout=60s
```

**Step 5: Deploy Sample Rover**

```bash
# Install rover resources
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/rover?ref=main

# Verify rover is ready
kubectl wait --for=condition=Ready -n default--group-sample--team-sample rovers/rover-sample --timeout=60s
```

### Basic Testing

**Test 1: Check All Components**
```bash
# List all Control Plane custom resources
kubectl get environments,zones,groups,teams,rovers -A
```

**Test 2: Inspect Sample Resources**
```bash
# Check the sample environment
kubectl describe environment default -n default

# Check the sample zone
kubectl describe zone zone-a -n default

# Check the sample team
kubectl describe team group-sample--team-sample -n default

# Check the sample rover
kubectl describe rover rover-sample -n default--group-sample--team-sample
```

**Test 3: Verify Operator Logs**
```bash
# Check operator health (should show no errors)
kubectl logs -l control-plane=controller-manager -n admin-system --tail=10
kubectl logs -l control-plane=controller-manager -n organization-system --tail=10
kubectl logs -l control-plane=controller-manager -n rover-system --tail=10
```

## Development Workflow

### Working with Custom Resources

**Create a new environment:**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: admin.cp.ei.telekom.de/v1
kind: Environment
metadata:
  name: dev
  namespace: dev
spec:
  displayName: "Development Environment"
EOF
```

**Create a new zone:**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: admin.cp.ei.telekom.de/v1
kind: Zone
metadata:
  name: dev-zone
  namespace: dev
spec:
  displayName: "Development Zone"
  environment:
    name: dev
    namespace: dev
EOF
```

**Create a new group:**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: organization.cp.ei.telekom.de/v1
kind: Group
metadata:
  name: my-group
  namespace: dev
spec:
  displayName: "My Development Group"
  environment:
    name: dev
    namespace: dev
EOF
```

### Testing Changes

**Monitor resource status:**
```bash
# Watch resources being created/updated
kubectl get environments,zones,groups,teams,rovers -A -w

# Check specific resource conditions
kubectl describe <resource-type> <resource-name> -n <namespace>
```

**Debug issues:**
```bash
# Check operator logs for specific component
kubectl logs deployment/<component>-controller-manager -n <component>-system

# Check events for troubleshooting
kubectl get events --sort-by='.lastTimestamp' -A
```

### Common Development Tasks

**Reset sample environment:**
```bash
# Delete all sample resources
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/rover?ref=main
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/org?ref=main
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/admin?ref=main

# Recreate them
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/admin?ref=main
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/org?ref=main
kubectl apply -k https://github.com/telekom/controlplane/install/local/resources/rover?ref=main
```

**Update Control Plane version:**
```bash
# Pull latest changes
kubectl apply -k https://github.com/telekom/controlplane/install/local?ref=main

# Restart operators to pick up changes
kubectl rollout restart deployment -l control-plane=controller-manager -A
```

## Troubleshooting

### Common Local Setup Issues

#### 1. Pods Stuck in Pending State

**Symptoms:** Operators not starting, pods in `Pending` status
**Solution:**
```bash
# Check node resources
kubectl describe nodes

# Check if images are being pulled
kubectl describe pod <pod-name> -n <namespace>

# For local clusters, ensure sufficient resources
# Recommended: 4GB RAM, 2 CPU cores minimum
```

#### 2. CRDs Not Recognized

**Symptoms:** `error validating data: ValidationError(Environment.spec)`
**Solution:**
```bash
# Verify CRDs are installed
kubectl get crd | grep cp.ei.telekom.de

# If missing, reinstall
kubectl apply -k https://github.com/telekom/controlplane/install/local?ref=main
```

#### 3. Resources Stuck in Non-Ready State

**Symptoms:** Resources created but conditions show `False`
**Solution:**
```bash
# Check operator logs
kubectl logs deployment/<component>-controller-manager -n <component>-system

# Check resource status and events
kubectl describe <resource-type> <resource-name> -n <namespace>

# Verify dependencies are ready first (environment before zone, etc.)
```

#### 4. Network Issues in Local Clusters

**Symptoms:** Image pull errors, timeout issues
**Solution:**
```bash
# For Docker Desktop: Ensure it has internet access
# For minikube: Check driver and network settings
minikube status

# For kind: Verify cluster is healthy
kind get clusters
kubectl cluster-info --context kind-<cluster-name>
```

### Quick Fixes

**Restart all operators:**
```bash
kubectl rollout restart deployment -l control-plane=controller-manager -A
```

**Clean slate restart:**
```bash
# Remove all Control Plane resources
kubectl delete -k https://github.com/telekom/controlplane/install/local?ref=main

# Wait a moment, then reinstall
kubectl apply -k https://github.com/telekom/controlplane/install/local?ref=main
```

**Check system health:**
```bash
# Overall cluster health
kubectl get nodes
kubectl get pods -A | grep -v Running

# Control Plane specific health
kubectl get pods -A -l control-plane=controller-manager
kubectl get crd | grep cp.ei.telekom.de | wc -l  # Should show multiple CRDs
```

## Next Steps

### Explore the Control Plane

1. **Learn about components:** Review the [main README](../../README.md#components) to understand each operator
2. **Read component docs:** Check individual component READMEs: [Rover](../../rover/README.md), [Admin](../../admin/README.md), [API](../../api/README.md), [Gateway](../../gateway/README.md), [Identity](../../identity/README.md), [Approval](../../approval/README.md), [Organization](../../organization/README.md), [Application](../../application/README.md), [Secret Manager](../../secret-manager/README.md)
3. **Understand architecture:** Review the [Control Plane Architecture](../../README.md#architecture) and view the detailed [architecture diagram](../img/CP_Architecture.drawio.svg)

### Production Deployment

1. **Full installation:** Follow the [Installation Guide](./installation.md) for production setup
2. **Infrastructure setup:** Learn about cert-manager, trust-manager, and monitoring requirements
3. **Security considerations:** Understand RBAC, secrets management, and network policies

### Development and Contribution

1. **Component development:** Each component has its own development setup in component directories
2. **Testing:** Learn about running tests with `make test` in component directories
3. **Contributing:** Review [CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution guidelines

### Community and Support

- **Issues and questions:** [GitHub Issues](https://github.com/telekom/controlplane/issues)
- **Community contact:** [opensource@telekom.de](mailto:opensource@telekom.de)
- **Code of Conduct:** [CODE_OF_CONDUCT.md](../../CODE_OF_CONDUCT.md)
- **Contributing:** [CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution guidelines
- **Changelog:** [CHANGELOG.md](../../CHANGELOG.md) for release notes and updates

## Cleanup

When you're done with local development:

```bash
# Remove all Control Plane resources
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/rover?ref=main
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/org?ref=main
kubectl delete -k https://github.com/telekom/controlplane/install/local/resources/admin?ref=main
kubectl delete -k https://github.com/telekom/controlplane/install/local?ref=main

# Optional: Delete the entire local cluster
# kind delete cluster --name <cluster-name>
# minikube delete
```

---

**🎉 Congratulations!** You now have a working Control Plane environment for development and testing. Start exploring the components and building your first APIs!