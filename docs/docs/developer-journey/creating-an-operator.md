---
sidebar_position: 3
---

# Creating a Custom Operator

The Control Plane is built as a collection of domain operators. Each operator manages a set of Kubernetes Custom Resources and reconciles them to the desired state. Every operator follows the same **Controller + Handler** pattern provided by the [Common library](https://github.com/telekom/controlplane/tree/main/common), which handles the reconciliation lifecycle so you can focus on business logic.

This guide walks you through scaffolding a new operator and wiring it into the framework.

## Prerequisites

- **Go 1.25.8**
- **Kubebuilder 4.9.0** — see the [installation instructions](https://book.kubebuilder.io/quick-start#installation)
- **A working Control Plane installation** — see [Local Development](./local-development.md)
- **Familiarity with the architecture** — review the [Architecture Overview](../architecture/overview.md) before continuing

## Scaffolding a New Operator

Use Kubebuilder to generate the initial project structure:

```bash
mkdir my-operator && cd my-operator
kubebuilder init --domain cp.ei.telekom.de --repo github.com/telekom/controlplane/my-operator
kubebuilder create api --group mygroup --version v1 --kind MyResource
```

This creates:

| File / Directory | Purpose |
|------------------|---------|
| `PROJECT` | Kubebuilder project metadata |
| `main.go` | Operator entry point — sets up the manager and registers controllers |
| `api/v1/` | Go types for your CRD (the spec, status, and type definitions) |
| `internal/controller/` | Controller scaffold with an empty `Reconcile` method |
| `config/crd/` | Generated CRD manifests |
| `config/rbac/` | Generated RBAC roles and bindings |
| `Makefile` | Build, test, generate, and deploy targets |

At this point you have a working Kubernetes operator — it just does not do anything yet. The next sections explain how to integrate it with the Common library.

## The Common Library

The `common` module provides the building blocks that every Control Plane operator uses. Understanding these components is essential before you write any business logic.

### Object Interface

Every CRD type must implement the `Object` interface. This extends the standard Kubernetes object with condition support, which the framework uses to track reconciliation progress:

- **`GetConditions()`** — returns the list of status conditions on the resource
- **`SetCondition(condition)`** — adds or updates a condition on the resource

Kubebuilder generates most of the boilerplate for you. You only need to add a `Conditions` field to your status struct and implement the two methods above.

### Handler Interface

The `Handler` is the only interface your operator must implement. This is where all business logic lives:

- **`CreateOrUpdate(ctx, obj)`** — called when a resource is created or updated. Inspect the spec, create or update child resources, and return the desired status.
- **`Delete(ctx, obj)`** — called when a resource is being deleted (i.e., it has a deletion timestamp). Clean up any external state and remove finalizers.

Everything else — watching resources, managing finalizers, updating conditions — is handled by the framework.

### Controller

The generic `Controller` wraps your handler and manages the full reconciliation lifecycle. When a resource changes, the controller:

1. Adds a finalizer to the resource (if not present)
2. Detects the virtual environment from the resource's labels
3. Injects a `ScopedClient` into the context so your handler can use it
4. Calls `CreateOrUpdate` or `Delete` on your handler depending on the resource state
5. Updates the status conditions based on the handler's return value
6. Handles requeue scheduling with jitter (default approximately 30 minutes)
7. Removes the finalizer when deletion is complete

You do not need to implement any of this yourself.

### ScopedClient

The `ScopedClient` is an environment-aware Kubernetes client. It automatically labels every resource it creates and scopes all queries to the correct virtual environment. When your handler creates child resources through the `ScopedClient`, they are automatically tagged with the right environment label.

### JanitorClient

The `JanitorClient` extends `ScopedClient` with garbage collection. It tracks which resources were created or updated during a reconciliation cycle and, once reconciliation completes, cleans up any orphaned resources that were not touched. This is useful for operators that create a dynamic set of child resources where the exact set may change between reconciliation runs.

## Implementing the Reconciler

The reconciler is the glue between Kubebuilder's controller-runtime and the Common library. A typical implementation looks like this:

```go
type MyResourceReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    controller.Controller[*v1.MyResource]
}

func (r *MyResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    r.Recorder = mgr.GetEventRecorderFor("myresource-controller")
    handler := &MyResourceHandler{}
    r.Controller = controller.NewController(handler, r.Client, r.Recorder)
    return ctrl.NewControllerManagedBy(mgr).For(&v1.MyResource{}).Complete(r)
}

func (r *MyResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    return r.Controller.Reconcile(ctx, req, &v1.MyResource{})
}
```

Here is what each part does:

- **`MyResourceReconciler`** — embeds both the Kubernetes client and the generic `Controller`. The generic type parameter tells the framework which CRD type this controller manages.
- **`SetupWithManager`** — called once at startup. It creates the handler (your business logic), wraps it in a `Controller`, and registers the reconciler with controller-runtime.
- **`Reconcile`** — called by controller-runtime whenever a resource changes. It delegates to the Common library's `Controller.Reconcile`, which handles the full lifecycle described above and calls your handler at the right time.

Your actual business logic goes into the `MyResourceHandler`, which implements the `Handler` interface.

## Error Handling

The Common library provides specialized error types that control how the framework responds to failures:

| Error Type | Behavior |
|------------|----------|
| **BlockedError** | The resource cannot proceed because of an unmet dependency. Sets a Blocked condition on the resource and requeues at the normal interval. |
| **RetryableError** | A temporary failure occurred (e.g., a transient network error). The resource is requeued quickly, approximately 1 second by default. |
| **RetryableWithDelayError** | A temporary failure with a custom retry delay. Lets you control exactly how long to wait before the next attempt. |

Each type has a convenience constructor:

```go
handler.BlockedErrorf("waiting for %s to be ready", dep.Name)
handler.RetryableErrorf("failed to reach external service: %v", err)
handler.RetryableWithDelayErrorf(5*time.Second, "rate limited, retrying in 5s")
```

Any other error returned from a handler is treated as an unexpected failure. The framework logs it and requeues using the default error interval.

## Condition System

All Control Plane resources use a standardized condition system to communicate their state. There are two condition types:

**Processing** — indicates whether reconciliation is in progress:

| Constructor | Meaning |
|-------------|---------|
| `NewProcessingCondition(reason)` | Reconciliation is actively running |
| `NewDoneProcessingCondition()` | Reconciliation completed successfully |
| `NewBlockedCondition(reason)` | Reconciliation is blocked by an external dependency |

**Ready** — indicates whether the resource is fully operational:

| Constructor | Meaning |
|-------------|---------|
| `NewReadyCondition()` | The resource is ready and healthy |
| `NewNotReadyCondition(reason)` | The resource is not ready |

The framework updates these conditions automatically based on the handler's return value and any errors it produces. You can also set conditions explicitly in your handler if you need more fine-grained control.

## Virtual Environments

Every resource in the Control Plane must carry a `cp.ei.telekom.de/environment` label that identifies the virtual environment it belongs to. The `ScopedClient` enforces this automatically — it reads the environment from the parent resource's labels and applies it to all child resources.

If a resource is missing this label, the controller sets it to **Blocked** status and does not proceed with reconciliation. This prevents resources from being reconciled outside of a defined environment context.

For more details on how virtual environments work, see the [Architecture Overview](../architecture/overview.md#virtual-environments).

## Configuration

The Common library uses [Viper](https://github.com/spf13/viper) for configuration. All settings can be overridden via environment variables:

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `requeue-after` | `REQUEUE_AFTER` | `30m` | Normal requeue interval after a successful reconciliation |
| `requeue-after-on-error` | `REQUEUE_AFTER_ON_ERROR` | `1s` | Requeue interval after a retryable error |
| `jitter-factor` | `JITTER_FACTOR` | `0.7` | Randomisation factor applied to requeue intervals to avoid thundering herds |
| `max-concurrent-reconciles` | `MAX_CONCURRENT_RECONCILES` | `10` | Maximum number of concurrent reconciliations per controller |

## Testing

Each operator follows the same testing approach used across the project:

- **Unit tests** — use [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) to run a lightweight Kubernetes API server in-process. Run with:
  ```bash
  make test
  ```

- **End-to-end tests** — use [Ginkgo](https://onsi.github.io/ginkgo/) to run tests against a real Kind cluster. Run with:
  ```bash
  make test-e2e
  ```

See [Local Development](./local-development.md#running-tests) for more details on the test setup and available Make targets.

## Next Steps

- Review existing operators in the repository (such as `api/`, `gateway/`, or `application/`) for real-world patterns and conventions
- Read the [Architecture Overview](../architecture/overview.md) for a deeper understanding of how domains interact
- Set up your [Local Development](./local-development.md) environment to run and test your operator
