# Migrator - Plugin-Based Migration Framework

A flexible, plugin-based Kubernetes operator framework for migrating resources from legacy clusters to new clusters.

## Overview

The Migrator module provides a generic framework for creating resource migration plugins. It eliminates code duplication by providing a reusable controller infrastructure while allowing each resource type to define its own migration logic.

**Key Benefits:**
- ✅ **Pluggable Architecture** - Add new resource types without modifying core framework
- ✅ **No Code Duplication** - Generic controller handles all resources
- ✅ **Type-Safe** - Interface enforces required methods
- ✅ **Easy to Extend** - Just implement the interface and register
- ✅ **Testable** - Mock the interface for isolated testing
- ✅ **Discoverable** - Registry shows all registered migrators

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Migrator Module                       │
│                                                           │
│  ┌───────────────────────────────────────────────────┐  │
│  │          Framework (pkg/framework/)               │  │
│  │  ┌─────────────┐  ┌──────────┐  ┌─────────────┐  │  │
│  │  │ Interfaces  │  │Registry  │  │  Generic    │  │  │
│  │  │             │  │          │  │  Controller │  │  │
│  │  └─────────────┘  └──────────┘  └─────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│                                                           │
│  ┌───────────────────────────────────────────────────┐  │
│  │        Migrators (internal/migrators/)            │  │
│  │  ┌──────────────┐  ┌────────────┐  ┌──────────┐  │  │
│  │  │  Approval    │  │   Rover    │  │   ...    │  │  │
│  │  │  Request     │  │            │  │          │  │  │
│  │  └──────────────┘  └────────────┘  └──────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
│                                                           │
│  ┌───────────────────────────────────────────────────┐  │
│  │              Main (cmd/main.go)                   │  │
│  │         - Registers all migrators                 │  │
│  │         - Sets up manager                         │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured
- Access to both legacy and new clusters
- Service account token from the legacy cluster

### Deploy

```bash
# Build
cd migrator
make docker-build docker-push IMG=<your-registry>/migrator:latest

# Deploy
make deploy IMG=<your-registry>/migrator:latest
```

### Configuration

Create the remote cluster secret (same as migration module):

```bash
kubectl create secret generic remote-cluster-token \
  --namespace=controlplane-system \
  --from-literal=server=https://legacy-cluster.example.com:6443 \
  --from-literal=token=<token-from-legacy-cluster> \
  --from-file=ca.crt=/path/to/legacy-cluster-ca.crt
```

## Creating a New Migrator

### Step 1: Implement the Interface

Create `internal/migrators/yourresource/migrator.go`:

```go
package yourresource

import (
    "context"
    "time"
    
    "github.com/telekom/controlplane/migrator/pkg/framework"
    yourv1 "github.com/telekom/controlplane/your-module/api/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type YourResourceMigrator struct {
    // Add fields as needed
}

func NewYourResourceMigrator() *YourResourceMigrator {
    return &YourResourceMigrator{}
}

// Implement framework.ResourceMigrator interface

func (m *YourResourceMigrator) GetName() string {
    return "yourresource"
}

func (m *YourResourceMigrator) GetNewResourceType() client.Object {
    return &yourv1.YourResource{}
}

func (m *YourResourceMigrator) GetLegacyAPIGroup() string {
    return "legacy.api.group.com"
}

func (m *YourResourceMigrator) ComputeLegacyIdentifier(
    ctx context.Context,
    obj client.Object,
) (namespace, name string, skip bool, err error) {
    // Compute legacy namespace and name
    // Return skip=true if migration should be skipped
    
    yourResource := obj.(*yourv1.YourResource)
    
    // Example: strip environment prefix from namespace
    legacyNamespace := stripEnvironment(yourResource.Namespace)
    legacyName := yourResource.Name
    
    return legacyNamespace, legacyName, false, nil
}

func (m *YourResourceMigrator) FetchFromLegacy(
    ctx context.Context,
    remoteClient client.Client,
    namespace, name string,
) (client.Object, error) {
    // Fetch the legacy resource
    legacyResource := &yourv1.LegacyYourResource{}
    key := client.ObjectKey{Namespace: namespace, Name: name}
    
    if err := remoteClient.Get(ctx, key, legacyResource); err != nil {
        return nil, err
    }
    
    return legacyResource, nil
}

func (m *YourResourceMigrator) HasChanged(
    ctx context.Context,
    current, legacy client.Object,
) bool {
    // Check if migration is needed
    // Compare states, check annotations, etc.
    
    yourResource := current.(*yourv1.YourResource)
    legacyResource := legacy.(*yourv1.LegacyYourResource)
    
    return yourResource.Spec.State != legacyResource.Spec.State
}

func (m *YourResourceMigrator) ApplyMigration(
    ctx context.Context,
    current, legacy client.Object,
) error {
    // Apply legacy state to current resource
    
    yourResource := current.(*yourv1.YourResource)
    legacyResource := legacy.(*yourv1.LegacyYourResource)
    
    yourResource.Spec.State = legacyResource.Spec.State
    // Copy other fields as needed
    
    return nil
}

func (m *YourResourceMigrator) GetRequeueAfter() time.Duration {
    return 30 * time.Second
}
```

### Step 2: Register the Migrator

Add to `cmd/main.go`:

```go
import (
    "github.com/telekom/controlplane/migrator/internal/migrators/yourresource"
    yourv1 "github.com/telekom/controlplane/your-module/api/v1"
)

func init() {
    // ... existing scheme setup
    utilruntime.Must(yourv1.AddToScheme(scheme))
}

func main() {
    // ... existing setup
    
    // Register your migrator
    if err := registry.Register(yourresource.NewYourResourceMigrator()); err != nil {
        setupLog.Error(err, "failed to register YourResource migrator")
        os.Exit(1)
    }
    
    // ... rest of main
}
```

### Step 3: Done!

That's it! The framework handles:
- ✅ Watching resources
- ✅ Reconciliation loop
- ✅ Logging
- ✅ Error handling
- ✅ Requeuing

## Framework Interface

```go
type ResourceMigrator interface {
    // GetName returns unique migrator name
    GetName() string
    
    // GetNewResourceType returns empty resource instance
    GetNewResourceType() client.Object
    
    // GetLegacyAPIGroup returns legacy API group
    GetLegacyAPIGroup() string
    
    // ComputeLegacyIdentifier computes namespace/name
    // Return skip=true to skip migration
    ComputeLegacyIdentifier(ctx context.Context, obj client.Object) (namespace, name string, skip bool, err error)
    
    // FetchFromLegacy fetches legacy resource
    FetchFromLegacy(ctx context.Context, remoteClient client.Client, namespace, name string) (client.Object, error)
    
    // HasChanged checks if migration needed
    HasChanged(ctx context.Context, current, legacy client.Object) bool
    
    // ApplyMigration applies legacy state
    ApplyMigration(ctx context.Context, current, legacy client.Object) error
    
    // GetRequeueAfter returns requeue duration
    GetRequeueAfter() time.Duration
}
```

## Built-in Migrators

### ApprovalRequest Migrator

Migrates ApprovalRequest resources from legacy `acp.ei.telekom.de/v1` API to new `approval.cp.ei.telekom.de/v1` API.

**Features:**
- Namespace transformation: `environment--group--team` → `group--team`
- Name component swapping: `rover--api` → `api--rover`
- State mapping: `SUSPENDED` → `Rejected`
- Change detection via annotations

## Development

### Project Structure

```
migrator/
├── cmd/
│   └── main.go                     # Entry point
├── pkg/
│   └── framework/
│       ├── interfaces.go           # ResourceMigrator interface
│       ├── controller.go           # Generic controller
│       └── registry.go             # Migrator registry
├── internal/
│   └── migrators/
│       ├── approvalrequest/        # ApprovalRequest migrator
│       └── yourresource/           # Your migrator here
├── config/                         # Kubernetes manifests
├── Dockerfile
├── Makefile
└── README.md
```

### Testing

Create tests for your migrator:

```go
func TestYourResourceMigrator_ComputeLegacyIdentifier(t *testing.T) {
    migrator := NewYourResourceMigrator()
    
    resource := &yourv1.YourResource{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test",
            Namespace: "env--group--team",
        },
    }
    
    ns, name, skip, err := migrator.ComputeLegacyIdentifier(context.Background(), resource)
    
    assert.NoError(t, err)
    assert.False(t, skip)
    assert.Equal(t, "group--team", ns)
    assert.Equal(t, "test", name)
}
```

### Build

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run locally
make run
```

## Comparison: Migration vs Migrator

| Feature | Migration Module | Migrator Module |
|---------|-----------------|-----------------|
| Architecture | Single resource hardcoded | Plugin-based framework |
| Adding resources | Duplicate controller/handler | Implement interface + register |
| Code reuse | Minimal | Maximum (shared controller) |
| Maintainability | Harder (duplication) | Easier (single framework) |
| Testing | Per resource | Framework + per migrator |
| Use case | Quick single-resource migration | Multiple resources, extensibility |

## Migration Path

If you're currently using the `migration` module:

1. ✅ Keep using it - it works fine for single resources
2. ✅ Switch to `migrator` if you need to add more resource types
3. ✅ Both can coexist in the cluster (different operators)

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.

## License

Apache-2.0 - See [LICENSE](../LICENSE) for details.
