<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <img src="./docs/icon.jpeg" alt="Common Library logo" width="200">
  <h1 align="center">Common</h1>
</p>

<p align="center">
  The common module provides shared functionality for all Operators implemented in the Controlplane.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#getting-started">Getting Started</a> •
  <a href="#configuration">Configuration</a>
</p>


## About

This module provides shared functionality for all [Operators](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) implemented using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).
It contains the following components:

- **Controller**: The controller implements the reconciliation logic for a single custom resource (CR). It contains common logic for all controllers, such as handling finalizers, status updates, and error handling. It uses a **Handler** to process the CR.
- **Handler**: The handler is called by the controller to process the CR. It can either be a `deletion` or `createOrUpdate` request. The handler is responsible for reaching the desired state of the CR and needs to be implemented by the specific operator.
- **ScopedClient**: This client is a wrapper around the default kubebuilder client. It provides a simplified interface and is also context-aware. That means that it supports [virtual environments](#virtual-environments)
- **JanitorClient**: This client is an extension of the `ScopedClient`. It is used to clean up resources that are no longer needed.

In addition to these components, the module also provides some common utilities and interfaces that are used by the controllers and handlers. These include:

- **Condition**: This package provides a set of utilities for managing conditions on CRs. See [Conditions](#conditions) for more information.
- **Types**: This package provides some common types that are used by the controllers and handlers. These include the `Object` interface, which is an extension to the kubebuilder `Object` interface
- **Util**: This package provides some common utilities including context and labels
- **Config**: This package provides a set of default configurations that can be used by the controllers. See [Configuration](#configuration) for more information.

## Controller and Handler Flow

The controller contains common logic and are used by all operators:

1. **Fetch**: Get the CR that is referenced by the event. If its not found, just return.
2. **Environment Detection**: Get the environment from the CR. If it is not set, set the CR to blocked and return.
3. **Context Injection**: Inject the `ScopedClient` into the current context. This client is used to interact with the Kubernetes API and is aware of the environment.
4. **Handle**: Call the handler to process the CR. The handler is responsible for reaching the desired state of the CR. It can either be a `deletion` or `createOrUpdate` request.
5. **Error/Success**: If the handler returns an error, start the error-handling. If not, requeue the CR with a jitter.


The following diagram shows the flow of the controller and handler:

<div align="center">
    <img width="800" height="700" src="docs/overview.drawio.svg" />
</div>

## Virtual Environments

Each custom resource (CR) is located in an environment. The environment is determined by the controller by extracting it from the labels of the CR. If it is not set, the controller will set the CR to blocked and it will not be processed. 

This is done to virtualize a Kubernetes cluster and to run multiple controlplane instances in a single cluster.

## Conditions 

We use the conditions feature that Kubernetes offers. See [docs](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties) for more information.

Helper functions can be found in [pkg/condition/condition.go](pkg/condition/condition.go)

### Condition Types

The following condition types are supported ([pkg/condition/condition.go](pkg/condition/condition.go)):

- **Ready**: Indicates that the resource is ready and operational
  - **Status**: `True` = Ready, `False` = Not Ready, `Unknown` = Unknown state
  - **Reasons**: Custom per domain (e.g., "SubResourceProvisioned", "ConfigurationValid")
  
- **Processing**: Indicates that the resource is being processed or reconciled
  - **Status**:
    - `True` = Currently processing, 
    - `False` = Processing complete or blocked
  - Out-Of-The-Box supported **Reasons**: 
    - `Blocked` (when processing is blocked), 
    - `Done` (when processing is complete),




## Scripts

- **install_crds**: See [install_crds](scripts/install_crds/README.md) for more information


## Configuration

The common module provides a configuration system that allows operators to customize their behavior. 
The configuration is managed by the `config` package and uses [Viper](https://github.com/spf13/viper) for handling configuration values.

> [!NOTE] 
> The configuration system is optional. If you do not need any configuration, default values will be applied.

### Available Configuration Options

Take a look at the [./pkg/config/config.go](./pkg/config/config.go) file for all available configuration options and their default values.

### Using Configuration in Your Operator

To use the configuration in your operator, simply import the config package. 
The operator will automatically load the configuration values from the sources and do not need to be aware of viper.

> [!IMPORTANT]
> Operators are still able to provide their own configuration values as flags. However, these should be limited to default
> flags generated by kubebuilder such as `--metrics-bind-address`, `--health-probe-bind-address`, and `--leader-elect`.

Examples to use in `common` present configs: 
* [organization/internal/controller/group_controller.go](../organization/internal/controller/group_controller.go)

#### Add new custom configuration in a domain

Domains can additionally add custom configuration options that are not part of `common`.
Simply, add a new configuration key to the global [Viper](https://github.com/spf13/viper) configuration.

Take a look at the following example:
```go
package main

import "github.com/spf13/viper"

// use the viper package to set up your configuration
func main() {
	//....
	// Set up a default value for your configuration key
	key := "test-feature"
	viper.SetDefault(key, "defaultValue")

    // Bind the key to an environment variable
    viper.BindEnv(key, "TEST_FEATURE")
	//....
}
```

### Environment Variable Configuration

Environment variables provide a flexible way to configure operators in Kubernetes deployments. 
The environment variables follow the naming convention defined in [./pkg/config/config.go](./pkg/config/config.go),
mapping from internal configuration keys to uppercase environment variable names.

For Kubernetes deployments, you can use Kustomize's configMapGenerator to provide environment variables to the operators:

```yaml
configMapGenerator:
  - name: admin-env-config
    namespace: admin-system
    behavior: create
    options:
      disableNameSuffixHash: true
    envs:
      - config/admin.cnf
```

And then mount these environment variables in your deployment:

```yaml
patches:
  - target:
      kind: Deployment
      name: admin-controller-manager
      namespace: admin-system
    patch: |-
      - op: add
        path: /spec/template/spec/containers/0/envFrom
        value:
          - configMapRef:
              name: admin-env-config
```

The `admin.cnf` file should contain the environment variables in key-value pairs:
```text
REQUEUE_AFTER_ON_ERROR=1m
REQUEUE_AFTER=30m
JITTER_FACTOR=0.9
MAX_BACKOFF=3m
MAX_CONCURRENT_RECONCILES=3
```

## Getting Started

For any relevant commands, you can use the provided [Makefile](./Makefile) to build and test the project.
To get started, you need to follow the steps described in the [kubebuilder docs](https://book.kubebuilder.io/getting-started).

After scaffolding the project, you can use the common module:

```go
// This struct is generated by kubebuilder
type MyResourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller.Controller[*v1.MyResource] // We extend it using the controller
}

func (r *MyResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("myresource-controller")
  // Our handler implement the handler.Handler[Object] interface
  handler := &MyResourceHandler{}
	r.Controller = controller.NewController(handler, r.Client, r.Recorder) 

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.MyResource{}).
		Complete(r)
}

func (r *MyResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
  // In the Reconcile function that is required by the reconcile.Reconciler interface, we call the controller
  // to handle the request.
	return r.Controller.Reconcile(ctx, req, &v1.MyResource{})
}
```

All your business logic should be implemented in the `MyResourceHandler` struct (see [example](./pkg/handler/nop.go)). The controller will take care of the rest.
Additionally, your resource `MyResource` must implement the `Object` interface. 

