---
sidebar_position: 2
---

# Kubernetes & Operators

The Control Plane extensively uses Kubernetes and the Operator pattern to manage resources and implement business logic.

## Kubernetes Operators

Kubernetes operators are software extensions to Kubernetes that use custom resources to manage applications and their components. The Control Plane implements several operators:

- **API Operator** - Manages API definitions, exposures, and subscriptions
- **Gateway Operator** - Controls API gateway configuration and routing
- **Identity Operator** - Handles identity providers and client registrations

## Controller Runtime

The Control Plane uses the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library (v0.21.0) to build Kubernetes operators. This library provides high-level APIs for:

- Creating and managing controllers
- Working with custom resources
- Implementing reconciliation loops
- Managing events, predicates, and webhooks

Example controller implementation:

```go
func (r *FileManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("filemanager", req.NamespacedName)
    
    // Fetch the FileManager resource
    fileManager := &apiV1.FileManager{}
    if err := r.Get(ctx, req.NamespacedName, fileManager); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Implement controller logic
    if fileManager.Spec.Storage.Type == "s3" {
        // Configure S3 backend
        if err := r.configureS3Backend(ctx, fileManager); err != nil {
            log.Error(err, "Failed to configure S3 backend")
            return ctrl.Result{Requeue: true}, err
        }
    }
    
    // Update status
    fileManager.Status.Ready = true
    if err := r.Status().Update(ctx, fileManager); err != nil {
        log.Error(err, "Failed to update FileManager status")
        return ctrl.Result{Requeue: true}, err
    }
    
    return ctrl.Result{}, nil
}
```

## Custom Resources

The Control Plane defines several Custom Resource Definitions (CRDs) that extend the Kubernetes API. Examples include:

- **API** - Represents an API specification
- **Gateway** - Represents an API gateway instance
- **FileManager** - Represents a file storage service

Example CRD:

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: filemanagers.storage.cp.ei.telekom.de
spec:
  group: storage.cp.ei.telekom.de
  names:
    kind: FileManager
    plural: filemanagers
    singular: filemanager
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                storage:
                  type: object
                  properties:
                    type:
                      type: string
                      enum: ["s3"]
                    s3:
                      type: object
                      properties:
                        endpoint:
                          type: string
                        bucket:
                          type: string
            status:
              type: object
              properties:
                ready:
                  type: boolean
```

## Kubebuilder

[Kubebuilder](https://book.kubebuilder.io/) is used as a framework for building Kubernetes APIs using CRDs, controllers, and webhooks. It provides:

- Scaffolding for new API types and controllers
- Testing infrastructure
- RBAC manifests generation
- Webhooks for validation and defaulting

The Control Plane follows the Kubebuilder project structure:

- **api/v1/** - API type definitions
- **controllers/** - Controller implementations
- **config/** - Configuration for CRDs, RBAC, webhooks, etc.

## Deployment Model

The Control Plane components are deployed as standard Kubernetes resources:

- **Deployments** - For controller managers and API servers
- **Services** - For exposing APIs
- **ConfigMaps** - For configuration
- **Secrets** - For sensitive data
- **NetworkPolicies** - For network security