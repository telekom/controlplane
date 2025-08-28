---
sidebar_position: 1
---

# Quickstart Guide

This guide will help you quickly set up a development environment and deploy the controlplane on a local Kubernetes cluster. Follow these steps to get a working environment in minutes.

:::tip What you'll accomplish
By the end of this guide, you'll have:
- A running local Kubernetes cluster
- The controlplane components installed and running
- Sample resources deployed to verify functionality
:::

## Prerequisites

Before starting, ensure you have the following tools installed:

- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) - Kubernetes in Docker
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) - Kubernetes command-line tool
- [GitHub CLI](https://cli.github.com/) - For authentication with GitHub

## Setup Process

### Step 1: Prepare Local Kind Cluster

First, we'll create a kind cluster and prepare it for the controlplane installation.

```bash
# [optional] clone the controlplane repo
git clone --branch main https://github.com/telekom/controlplane.git
cd controlplane

# kind cluster
kind create cluster

# namespace
kubectl create namespace secret-manager-system

# gh-auth
export GITHUB_TOKEN=$(gh auth token)

# install
bash ./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds
```

### Step 2: Install Controlplane

Now that we have a Kubernetes cluster with the necessary dependencies, we can install the controlplane components. 
The controlplane consists of multiple controllers and custom resources that work together to manage workloads across 
Kubernetes clusters.

```bash
# Navigate to the installation directory
cd install/local

# Apply the Kustomization to install controlplane components
kubectl apply -k .

# Verify the installation by checking the controller-manager pods
kubectl get pods -A -l control-plane=controller-manager
```

You should see the controlplane controller pods running. The controllers are responsible for reconciling the custom 
resources and managing the platform.

If you want to develop and test your own controllers against the controlplane, please refer to the [Installation Guide](./installation.md) and the section "Development: Testing Local Controllers" for detailed instructions.

### Step 3: Install Sample Resources

To verify that the controlplane is working correctly and to understand its functionality, we'll deploy some sample 
resources. The controlplane uses a hierarchical resource model with admin, organization, and rover resources.

#### Admin Custom Resources

Admin resources define the infrastructure foundation, including zones, clusters, and platform-level configurations. 
These resources are typically managed by platform administrators.

```bash
kubectl apply -k resources/admin

# Verify
kubectl wait --for=condition=Ready -n controlplane zones/dataplane1
```

#### Organization Custom Resources

Organization resources represent teams and projects that use the platform. These resources define ownership, 
permissions, and organizational boundaries.

```bash
kubectl apply -k resources/org

# Verify
kubectl wait --for=condition=Ready -n controlplane teams/phoenix--firebirds
```

#### Rover Custom Resources

Rover resources represent the actual workloads that run on the platform. Rovers are deployable units that can be 
scheduled on different clusters based on requirements and zone capabilities.

```bash
kubectl apply -k resources/rover

# Verify the rover is ready
kubectl wait --for=condition=Ready -n controlplane--phoenix--firebirds rovers/rover-echo-v1
```

## Verification & Troubleshooting

### Expected Results

After successful installation, you should see:

- Controller pods running in the `controlplane` namespace
- Custom resources in `Ready` state
- A namespace created for the sample team (`controlplane--phoenix--firebirds`)

### Checking Resource Status

```bash
# Check all controlplane resources
kubectl get zones,teams,rovers -A

# Check that rover workloads are deployed
kubectl get pods -n controlplane--phoenix--firebirds
```

### Common Issues

If you encounter issues during the installation, here are some common troubleshooting steps:

- Check the controller logs:
  ```bash
  kubectl logs -n controlplane deploy/controlplane-controller-manager -c manager
  ```

- Verify that all custom resource definitions are installed:
  ```bash
  kubectl get crds | grep controlplane
  ```

- Ensure the correct namespace is created for your resources:
  ```bash
  kubectl get ns | grep controlplane
  ```

## Next Steps

Now that you have a working controlplane installation, you can:

1. **Explore the architecture**: Learn about the [controlplane components](../0-Overview/intro.md#components) and how they work together
2. **Deploy your own workloads**: Create custom [Rover resources](../3-Components/rover.md) for your applications
3. **Understand resource relationships**: See how [admin, organization and rover resources](../0-Overview/intro.md#features) interact
4. **Dive deeper**: Check out the [detailed installation guide](./installation.md) for more advanced options

For complete documentation, visit the [GitHub repository](https://github.com/telekom/controlplane).
```