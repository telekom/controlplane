---
sidebar_position: 1
---

# Quickstart Guide

This quickstart guide will help you set up a development environment and deploy the controlplane on a local Kubernetes 
cluster. The guide covers setting up a local kind cluster, installing the controlplane components, and deploying 
sample resources to verify the installation.

## Local Setup

The controlplane runs on Kubernetes. For local development and testing, we'll use [kind](https://kind.sigs.k8s.io/) (Kubernetes IN Docker) 
to create a lightweight local Kubernetes cluster. The following steps will guide you through setting up your local 
environment.

## Prerequisites

Make sure you have [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/), and the [GitHub CLI](https://cli.github.com/) installed before proceeding.

### Prepare local kind Cluster

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

### Install Controlplane

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

If you want to develop and test your own controllers against the controlplane, please refer to the 
[Installation Guide](https://github.com/telekom/controlplane/tree/main/docs/files/installation.md) and the section `Testing my local Controllers` for detailed instructions on how to run 
locally built controllers.

### Install Sample Resources

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

## Troubleshooting

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

1. Explore the custom resources and their relationships
2. Deploy your own Rover resources
3. Learn about zone capabilities and workload scheduling
4. Check out the detailed documentation for each component

For more information, refer to the other sections of the documentation or visit the [GitHub repository](https://github.com/telekom/controlplane).
```