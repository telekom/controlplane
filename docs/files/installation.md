# Installation 

This is an installation guide for the controlplane project. It will help you to set up the controlplane in a local 
Kubernetes cluster. If you already know how it works, you can switch to the [Quickstart Guide](https://github.com/telekom/controlplane/tree/main/docs/files/quickstart.md) 
to get started right away.

## Local Setup

You can deploy the controlplane to your local Kubernetes cluster, e.g. using [kind](https://kind.sigs.k8s.io/).

> **ℹ️ Important:** Before you start executing commands, please read the complete documentation for this chapter. 
> Otherwise, you might need to do some backtracking.

### Installing local kind Cluster

```bash
# [optional] clone the controlplane repo
git clone --branch main https://github.com/telekom/controlplane.git
cd controlplane

# [optional] create a new kind cluster
kind create cluster

# make sure you are connected to the correct Kubernetes context
kubectl config current-context
# > kind-kind

# Install the required components
## At the moment this install.sh depends on this namespace to exist, you need to create it manually
kubectl create namespace secret-manager-system
## Download the install.sh script and use it to install the components
## You might need to set the GITHUB_TOKEN variable to avoid the rate-limits
export GITHUB_TOKEN=$(gh auth token)
bash ./install.sh --with-cert-manager --with-trust-manager --with-monitoring-crds

# Now your local kind-cluster is setup with the required cert-manager, trust-manager and monitoring crds
```

### Installing the Controlplane

Have a look at the [docs](https://github.com/telekom/controlplane/tree/main/install/local) on how to install it locally.

```bash
cd install/local
kubectl apply -k .

#verify installation
kubectl get pods -A -l control-plane=controller-manager
```

### Creating the required Admin Resource

Navigate to the [example admin resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/admin). 
Adjust these resource as needed.

Install the admin resources to your local cluster:
```bash
kubectl apply -k resources/admin

# Verify
kubectl wait --for=condition=Ready -n controlplane zones/dataplane1
```

### Creating the required Organization Resource

Navigate to the [example organization resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/org). 
Adjust these resource as needed.

Install the organization resources to your local cluster:
```bash
kubectl apply -k resources/org

# Verify
kubectl wait --for=condition=Ready -n controlplane teams/phoenix--firebirds
```

### Creating the required Rover Resource

Navigate to the [example rover resource](https://github.com/telekom/controlplane/tree/main/install/local/resources/rover). 
Adjust these resource as needed.

Install the rover resources to your local cluster:
```bash
kubectl apply -k resources/rover

# Verify 
kubectl wait --for=condition=Ready -n controlplane--phoenix--firebirds rovers/rover-echo-v1
```

### Testing my local Controllers

To test your locally developed controller, you have multiple options that are described below.

#### Option 1

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
5. 🎉 Success: Your controller should now run with your local version


#### Option 2


> **⚠️ Important:** You cannot test any webhook logic by running the controller like this!

1. Before you run [Installing the Controlplane](#installing-the-controlplane), navigate to `install/local/kustomization.yaml`
2. Edit the file and comment out all controller that you want to test locally, e. g. if I want to test rover, api and gateway:
    ```yaml
    resources:
      - ../../secret-manager/config/default
      - ../../identity/config/default
    #  - ../../gateway/config/default
      - ../../approval/config/default
    #  - ../../rover/config/default
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
5. 🎉 Success: Your controllers should now run as a seperate process connected to the kind-cluster

