<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Installation

This guide provides comprehensive instructions for installing the Open Telekom Integration Platform Control Plane in a production Kubernetes environment. The Control Plane is a centralized management layer that orchestrates API lifecycle management, approval workflows, and secure service integration.

> **Quick Start:** For local development and testing, see the [Quickstart Guide](./quickstart.md) for a faster setup process.
> 
> **Architecture:** To understand the overall system design, review the [Control Plane Architecture](../../README.md#architecture) and [architecture diagram](../img/CP_Architecture.drawio.svg).

## Prerequisites

### System Requirements

- **Kubernetes**: Version 1.31.0 or higher
- **Operating System**: Linux, macOS, or Windows with WSL2
- **Network Access**: Internet connectivity for downloading container images and Helm charts
- **Cluster Resources**: Minimum 4 CPU cores and 8GB RAM available across cluster nodes

### Required Tools

Before installing the Control Plane, ensure you have the following tools installed:

- **kubectl**: Kubernetes command-line tool
  - Installation: [kubectl installation guide](https://kubernetes.io/docs/tasks/tools/)
  - Verify: `kubectl version --client`

- **helm**: Kubernetes package manager (version 3.0+)
  - Installation: [Helm installation guide](https://helm.sh/docs/intro/install/)
  - Verify: `helm version`

- **jq**: JSON processor for parsing API responses
  - Installation: [jq installation guide](https://jqlang.github.io/jq/download/)
  - Verify: `jq --version`

- **curl**: Command-line tool for transferring data
  - Usually pre-installed on most systems
  - Verify: `curl --version`

### Infrastructure Dependencies

The Control Plane requires the following infrastructure components to operate correctly:

#### Required Dependencies

- **cert-manager** (v1.17.2): Creates and manages TLS certificates
  - [cert-manager documentation](https://cert-manager.io/docs/)

- **trust-manager** (v0.17.1): Manages trust bundles in Kubernetes clusters
  - [trust-manager documentation](https://cert-manager.io/docs/trust/trust-manager/)

- **Prometheus Operator CRDs** (v20.0.0): Custom Resource Definitions for monitoring
  - [Prometheus Operator documentation](https://prometheus-operator.dev/)

#### Optional Dependencies

- **API Management Gateway**: Kong-based gateway for hybrid API management
  - [Gateway documentation](https://github.com/telekom/gateway-kong-charts)

- **Identity Management**: Keycloak-based M2M Identity Provider
  - [Iris documentation](https://github.com/telekom/identity-iris-keycloak-charts)

### Kubernetes Cluster Access

Ensure you have:
- Administrative access to your Kubernetes cluster
- Current kubectl context pointing to the target cluster
- Sufficient RBAC permissions to create cluster-wide resources

Verify your cluster access:
```bash
kubectl cluster-info
kubectl auth can-i create clusterroles --all-namespaces
```

## Production Installation

### Method 1: Automated Installation (Recommended)

The easiest way to install the Control Plane is using the provided installation script.

#### Download and Run Installation Script

```bash
# Download the installation script
curl -sSL https://raw.githubusercontent.com/telekom/controlplane/main/install.sh -o install.sh
chmod +x install.sh

# Run with all dependencies (recommended for new installations)
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

#### Installation Options

The installation script supports the following options:

- `--with-cert-manager`: Install cert-manager for TLS certificate management
- `--with-trust-manager`: Install trust-manager for certificate trust bundles
- `--with-monitoring-crds`: Install Prometheus Operator CRDs for monitoring
- `-h, --help`: Display help information

**Examples:**

```bash
# Minimal installation (dependencies must be pre-installed)
./install.sh

# Install with cert-manager and monitoring
./install.sh --with-cert-manager --with-monitoring-crds

# Install all optional dependencies
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

#### GitHub Token (Optional)

For better API rate limits, set a GitHub token:
```bash
export GITHUB_TOKEN=your_github_token_here
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

### Method 2: Manual Installation

For advanced users who prefer manual control over the installation process.

#### Step 1: Install Infrastructure Dependencies

**Install cert-manager:**
```bash
helm repo add jetstack https://charts.jetstack.io --force-update
helm upgrade cert-manager jetstack/cert-manager \
  --install \
  --namespace cert-manager \
  --create-namespace \
  --version v1.17.2 \
  --set crds.enabled=true \
  --wait
```

**Install trust-manager:**
```bash
helm upgrade trust-manager jetstack/trust-manager \
  --install \
  --namespace cert-manager \
  --version v0.17.1 \
  --set app.trust.namespace=secret-manager-system \
  --wait
```

**Install Prometheus Operator CRDs:**
```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
helm upgrade prometheus-operator-crds prometheus-community/prometheus-operator-crds \
  --install \
  --namespace monitoring \
  --create-namespace \
  --version v20.0.0 \
  --wait
```

#### Step 2: Install Control Plane

```bash
# Download the kustomization file
curl -sSL https://raw.githubusercontent.com/telekom/controlplane/v0.7.0/install/kustomization.yaml -o kustomization.yaml

# Apply the Control Plane components
kubectl apply -k .
```

## Configuration

### Environment Variables

The installation script accepts the following environment variables:

- `GITHUB_TOKEN`: GitHub personal access token for API rate limiting
- `CONTROLPLANE_VERSION`: Specific version to install (default: "latest")

### Customization Options

#### Custom Kubernetes Context

The installation script will prompt you to select the Kubernetes context:
```bash
Install on which Kubernetes context? [current-context]: your-cluster-context
```

#### Version Selection

To install a specific version:
```bash
export CONTROLPLANE_VERSION=v0.7.0
./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

#### Namespace Configuration

The Control Plane components are installed in the following namespaces:
- `cert-manager`: cert-manager and trust-manager components
- `secret-manager-system`: Secret management components
- `monitoring`: Prometheus Operator CRDs
- Component-specific namespaces for each operator

## Verification

### Check Installation Status

After installation, verify that all components are running:

```bash
# Check all Control Plane pods
kubectl get pods -A -l control-plane=controller-manager

# Check specific component namespaces
kubectl get pods -n secret-manager-system
kubectl get pods -n cert-manager

# Verify CRDs are installed
kubectl get crd | grep -E "(cp\.ei\.telekom\.de|stargate\.cp\.ei\.telekom\.de)"
```

### Health Checks

**Verify cert-manager:**
```bash
kubectl get pods -n cert-manager
kubectl get certificates -A
```

**Verify Control Plane operators:**
```bash
# Check operator deployments
kubectl get deployments -A -l control-plane=controller-manager

# Check operator logs (replace with specific operator name)
kubectl logs -n <namespace> deployment/<operator-name>-controller-manager
```

**Test basic functionality:**
```bash
# Check if CRDs are properly registered
kubectl explain environments
kubectl explain zones
kubectl explain rovers
```

### Expected Output

A successful installation should show:
- All operator pods in `Running` state
- CRDs properly registered and accessible
- No error messages in operator logs
- cert-manager certificates in `Ready` state

## Troubleshooting

### Common Issues

#### 1. RBAC Permission Errors

**Symptoms:** Permission denied errors during installation
**Solution:**
```bash
# Ensure you have cluster-admin privileges
kubectl auth can-i create clusterroles --all-namespaces

# If using managed Kubernetes, ensure your user has sufficient permissions
```

#### 2. Image Pull Errors

**Symptoms:** Pods stuck in `ImagePullBackOff` state
**Solution:**
```bash
# Check if images are accessible
kubectl describe pod <pod-name> -n <namespace>

# Verify network connectivity to ghcr.io
curl -I https://ghcr.io
```

#### 3. CRD Installation Failures

**Symptoms:** Custom resources not recognized
**Solution:**
```bash
# Manually apply CRDs
kubectl apply -f https://raw.githubusercontent.com/telekom/controlplane/v0.7.0/install/kustomization.yaml

# Check CRD status
kubectl get crd | grep cp.ei.telekom.de
```

#### 4. Dependency Installation Issues

**Symptoms:** cert-manager or other dependencies failing
**Solution:**
```bash
# Check Helm repository status
helm repo list
helm repo update

# Reinstall dependencies manually
helm uninstall cert-manager -n cert-manager
# Then reinstall following the manual installation steps
```

### Getting Help

If you encounter issues not covered in this troubleshooting section:

1. **Check the logs:**
   ```bash
   kubectl logs -n <namespace> deployment/<component>-controller-manager
   ```

2. **Review the GitHub Issues:**
   - [Control Plane Issues](https://github.com/telekom/controlplane/issues)

3. **Contact the community:**
   - Email: [opensource@telekom.de](mailto:opensource@telekom.de)
   - Review our [Code of Conduct](../../CODE_OF_CONDUCT.md)
   - Check [Contributing Guidelines](../../CONTRIBUTING.md)

4. **Documentation:**
   - [Component-specific documentation](../../README.md#components)
   - [Architecture overview](../../README.md#architecture) and [architecture diagram](../img/CP_Architecture.drawio.svg)
   - Individual component READMEs: [Rover](../../rover/README.md), [Admin](../../admin/README.md), [API](../../api/README.md), [Gateway](../../gateway/README.md), [Identity](../../identity/README.md), [Approval](../../approval/README.md), [Organization](../../organization/README.md), [Application](../../application/README.md), [Secret Manager](../../secret-manager/README.md)
   - [Local development setup](./quickstart.md) for testing and development

### Diagnostic Commands

```bash
# Comprehensive system check
kubectl get nodes
kubectl get pods -A
kubectl get crd | grep -E "(cp\.ei\.telekom\.de|stargate\.cp\.ei\.telekom\.de)"
kubectl get deployments -A -l control-plane=controller-manager

# Resource usage check
kubectl top nodes
kubectl top pods -A
```

## Next Steps

After successful installation:

1. **Set up your first environment:** Follow the [Quickstart Guide](./quickstart.md) for local development
2. **Explore components:** Review individual [component documentation](../../README.md#components)
3. **Understand the architecture:** Study the [Control Plane Architecture](../../README.md#architecture) and view the [architecture diagram](../img/CP_Architecture.drawio.svg)
4. **Configure API management:** Set up your first API exposure and subscription using the [API Operator](../../api/README.md)
5. **Set up organizations:** Configure groups and teams with the [Organization Operator](../../organization/README.md)
6. **Monitor your installation:** Configure monitoring and alerting
7. **Join the community:** Connect with other users and contributors through [opensource@telekom.de](mailto:opensource@telekom.de)

## Uninstallation

To remove the Control Plane:

```bash
# Remove Control Plane components
kubectl delete -k .

# Remove infrastructure dependencies (optional)
helm uninstall cert-manager -n cert-manager
helm uninstall trust-manager -n cert-manager
helm uninstall prometheus-operator-crds -n monitoring

# Remove namespaces
kubectl delete namespace cert-manager monitoring secret-manager-system
```

**Warning:** This will remove all Control Plane resources and data. Ensure you have backups if needed.