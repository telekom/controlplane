---
sidebar_position: 2
---

# Installation Guide

This guide provides detailed instructions for setting up the Open Telekom Integration Platform controlplane in a local Kubernetes environment. For a condensed version, see our [Quickstart Guide](../2-Installation/quickstart.md).

## Prerequisites

Before beginning the installation, ensure you have the following tools installed:

- [Git](https://git-scm.com/downloads)
- [Docker](https://docs.docker.com/get-docker/)
- [Kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) (Kubernetes IN Docker)
- [GitHub CLI](https://cli.github.com/) (for authentication)
- [Ko](https://ko.build/install/) (only required for local development)

## Installation Overview

The installation process consists of four main steps:

1. Setting up a local Kubernetes cluster
2. Installing the controlplane components
3. Creating the required resources
4. Verifying the installation

## Local Environment Setup

The controlplane runs on Kubernetes. For local development and testing, we'll use [kind](https://kind.sigs.k8s.io/) (Kubernetes IN Docker) to create a lightweight local Kubernetes cluster.

:::caution
Please read through the entire section before executing commands to ensure proper setup and avoid potential issues.
:::

### Step 1: Set Up a Local Kind Cluster

```bash
# Clone the repository (if you haven't already)
git clone --branch main https://github.com/telekom/controlplane.git
cd controlplane

# Create a new Kind cluster
kind create cluster

# Verify you're connected to the correct Kubernetes context
kubectl config current-context
# Output should be: kind-kind

# Create the required namespace
kubectl create namespace secret-manager-system

# Set GitHub token to avoid rate limits
export GITHUB_TOKEN=$(gh auth token)

# Install required components with the script
bash ./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds

# Now your local kind-cluster is set up with the required components:
# - cert-manager (for certificate management)
# - trust-manager (for trust bundle management)
# - monitoring CRDs (for Prometheus monitoring)
```

### Step 2: Install the Controlplane

The controlplane consists of multiple controllers and custom resources that manage workloads across Kubernetes clusters.

Follow these steps to install the controlplane components:

```bash
# Navigate to the installation directory
cd install/local

# Apply the kustomization to install controlplane components
kubectl apply -k .

# Verify installation by checking that controller pods are running
kubectl get pods -A -l control-plane=controller-manager
```

### Step 3: Create Required Resources

#### 3.1 Create Admin Resources

Navigate to the [example admin resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/admin). 
Adjust these resource as needed.

Install the admin resources to your local cluster:
```bash
# Apply the admin resources
kubectl apply -k resources/admin

# Verify that the zone is ready
kubectl wait --for=condition=Ready -n controlplane zones/dataplane1
```

#### 3.2 Create Organization Resources

Navigate to the [example organization resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/org). 
Adjust these resource as needed.

Install the organization resources to your local cluster:
```bash
# Apply the organization resources
kubectl apply -k resources/org

# Verify that the team is ready
kubectl wait --for=condition=Ready -n controlplane teams/phoenix--firebirds
```

#### 3.3 Create Rover Resources

Navigate to the [example rover resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/rover). 
Adjust these resource as needed.

Install the rover resources to your local cluster:
```bash
# Apply the rover resources
kubectl apply -k resources/rover

# Verify that the rover is ready
kubectl wait --for=condition=Ready -n controlplane--phoenix--firebirds rovers/rover-echo-v1
```

## Development: Testing Local Controllers

For developers who want to test their own controller implementations, there are two approaches available:

To test your locally developed controller, you have multiple options that are described below.

### Option 1: Replace Controller Images

You now have the `stable` release deployed on your local kind-cluster. To change specific versions of controllers, you 
can do:

1. Navigate to `install/local/kustomization.yaml`
2. Build the controller you want to test:
    ```bash
    # Example for rover controller
    cd rover
    
    export KO_DOCKER_REPO=ko.local
    ko build --bare cmd/main.go --tags rover-test
    kind load docker-image ko.local:rover-test
    ```
3. Update `install/local/kustomization.yaml` with the new rover-controller image
4. Install again:
    ```bash
    # [optional] navigate back to the install/local directory
    cd install/local
    # Apply the changes
    kubectl apply -k .
    ```
5. **Success!** Your controller is now running with your local version


### Option 2: Run Controllers Locally

> [!WARNING]
> You cannot test any webhook logic by running the controller like this!

1. Before you run [Install the Controlplane](#step-2-install-the-controlplane), navigate to `install/local/kustomization.yaml`
2. Edit the file and comment out all controller that you want to test locally, e.g. if I want to test rover, api and gateway:
    ```yaml
    resources:
      - ../../secret-manager/config/default
      - ../../identity/config/default
      # - ../../gateway/config/default
      - ../../approval/config/default
      # - ../../rover/config/default
      - ../../application/config/default
      - ../../organization/config/default
      # - ../../api/config/default
      - ../../admin/config/default
    ```
3. Apply it using `kubectl apply -k .`
4. Now, the rover-, api- and gateway-controllers were not deployed. You need to do it manually
    ```bash
    # forEach controller
    cd rover
    make install # install the CRDs
    export ENABLE_WEBHOOKS=false # if applicable, disable webhooks
    go run cmd/main.go # start the controller process
    ```
5. **Success!** Your controllers are now running as separate processes connected to the kind-cluster

## Troubleshooting

If you encounter issues during installation, here are some common troubleshooting steps:

### Checking Controller Logs

```bash
kubectl logs -n controlplane deploy/controlplane-controller-manager -c manager
```

### Verifying CRD Installation

```bash
kubectl get crds | grep controlplane
```

### Checking Resource Status

```bash
kubectl get zones,teams,rovers -A
```

## Next Steps

After successful installation, you can:

1. Explore the created resources using `kubectl get` commands
2. Deploy your own workloads using Rover resources
3. Learn about the controlplane architecture and components
4. Test API endpoints for resource management

