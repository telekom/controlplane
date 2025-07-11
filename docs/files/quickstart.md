# Quickstart Guide

## Local Setup

### Prepare local kind Cluster

```bash
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

```bash
# cd install/local
kubectl apply -k .

#verify installation
kubectl get pods -A -l control-plane=controller-manager
```

If you want to use local controllers to test the controlplane, please have a look at the 
[Installation Guide](https://github.com/telekom/controlplane/tree/main/docs/files/installation.md) and the section
`Testing my local Controllers`.

### Install sample resources

**admin custom resources**
```bash
kubectl apply -k resources/admin

# Verify
kubectl wait --for=condition=Ready -n controlplane zones/dataplane1
```

**organization custom resources**
```bash
kubectl apply -k resources/org

# Verify
kubectl wait --for=condition=Ready -n controlplane teams/phoenix--firebirds
```

**rover custom resources**
```bash
kubectl apply -k resources/rover

# Verify 
kubectl wait --for=condition=Ready -n controlplane--phoenix--firebirds rovers/rover-echo-v1
```