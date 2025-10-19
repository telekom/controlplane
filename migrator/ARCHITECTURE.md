# Migrator Architecture

## Overview

The Migrator uses a **Plugin-based Handler Architecture** that combines the flexibility of plugins with the testability of handlers.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────┐
│                     Entry Point (main.go)                    │
│  - Creates Manager                                           │
│  - Registers Migrators (plugins)                             │
│  - Sets up Generic Controller                                │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│              Framework Layer (pkg/framework/)                │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Generic Controller                                   │  │
│  │  - Reconciles all resource types                     │  │
│  │  - Delegates to ResourceMigrator                     │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ResourceMigrator Interface                          │  │
│  │  - GetName()                                         │  │
│  │  - GetNewResourceType()                              │  │
│  │  - GetLegacyAPIGroup()                               │  │
│  │  - ComputeLegacyIdentifier()                         │  │
│  │  - FetchFromLegacy()                                 │  │
│  │  - HasChanged()                                      │  │
│  │  - ApplyMigration()                                  │  │
│  │  - GetRequeueAfter()                                 │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Registry                                             │  │
│  │  - Register(migrator)                                │  │
│  │  - SetupAll(manager)                                 │  │
│  └──────────────────────────────────────────────────────┘  │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│        Plugin Layer (internal/migrators/*/migrator.go)      │
│                                                              │
│  Each plugin implements ResourceMigrator interface:         │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ApprovalRequestMigrator (Orchestrator)              │  │
│  │  - Implements interface methods                      │  │
│  │  - Delegates to handler                              │  │
│  │  - Type conversions (interface → concrete types)     │  │
│  └──────────────────┬───────────────────────────────────┘  │
│                     │                                        │
│                     ▼                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  MigrationHandler (Business Logic)                   │  │
│  │  - ComputeLegacyIdentifier()                         │  │
│  │  - FetchFromLegacy()                                 │  │
│  │  - HasChanged()                                      │  │
│  │  - ApplyMigration()                                  │  │
│  │  - Helper methods                                    │  │
│  └──────────────────┬───────────────────────────────────┘  │
│                     │                                        │
│                     ▼                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ApprovalMapper (State Mapping)                      │  │
│  │  - MapState() - Suspended → Rejected                │  │
│  │  - MapApprovalToRequest()                            │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
migrator/
├── cmd/
│   └── main.go                          # Entry point with registry
│
├── pkg/
│   └── framework/
│       ├── interfaces.go                # ResourceMigrator interface
│       ├── controller.go                # Generic reconciler
│       └── registry.go                  # Plugin registry
│
└── internal/
    └── migrators/
        └── approvalrequest/
            ├── migrator.go              # Plugin orchestrator
            ├── handler.go               # Business logic ⭐ NEW
            ├── handler_test.go          # Handler tests ⭐ NEW
            └── mapper.go                # State mapping
```

## Component Responsibilities

### 1. Generic Controller (`pkg/framework/controller.go`)
**Responsibility:** Resource type-agnostic reconciliation loop

- Watches resources based on migrator type
- Calls migrator interface methods
- Handles requeue logic
- **Does not** know about specific resource types

### 2. ResourceMigrator Interface (`pkg/framework/interfaces.go`)
**Responsibility:** Contract for all migrators

- Defines plugin API
- Ensures consistency across migrators
- Enables registry system

### 3. Migrator (e.g., `approvalrequest/migrator.go`)
**Responsibility:** Plugin orchestration and interface implementation

```go
// Orchestrator - implements ResourceMigrator interface
type ApprovalRequestMigrator struct {
    handler *MigrationHandler  // Delegates to handler
}

// Interface method - type conversion + delegation
func (m *ApprovalRequestMigrator) ComputeLegacyIdentifier(
    ctx context.Context,
    obj client.Object,  // Generic interface type
) (namespace, name string, skip bool, err error) {
    // Convert interface to concrete type
    approvalRequest := obj.(*approvalv1.ApprovalRequest)
    
    // Delegate to handler with concrete types
    return m.handler.ComputeLegacyIdentifier(ctx, approvalRequest)
}
```

**Responsibilities:**
- ✅ Implements ResourceMigrator interface
- ✅ Type conversion (interface → concrete types)
- ✅ Delegates to handler
- ❌ NO business logic

### 4. Handler (e.g., `approvalrequest/handler.go`) ⭐ **NEW**
**Responsibility:** Business logic with concrete types

```go
// Business logic handler - works with concrete types
type MigrationHandler struct {
    mapper *ApprovalMapper
    log    logr.Logger
}

// Business logic method - concrete types
func (h *MigrationHandler) ComputeLegacyIdentifier(
    ctx context.Context,
    approvalRequest *approvalv1.ApprovalRequest,  // Concrete type
) (namespace, name string, skip bool, err error) {
    // All business logic here:
    // - Check strategy
    // - Parse owner references
    // - Compute legacy names
    // - etc.
}
```

**Responsibilities:**
- ✅ All business logic
- ✅ Works with concrete types (easier to code)
- ✅ Easy to test (no interface mocking needed)
- ✅ Logging and error handling

### 5. Registry (`pkg/framework/registry.go`)
**Responsibility:** Plugin management

- Register migrators dynamically
- Setup controllers for all registered migrators
- Enable/disable migrators

## Data Flow Example

### ApprovalRequest Migration Flow

```
1. Watch Event
   └─> Generic Controller receives ApprovalRequest change

2. Controller calls Migrator Interface
   └─> ComputeLegacyIdentifier(ctx, obj client.Object)

3. Migrator (Orchestrator) converts types
   └─> approvalRequest := obj.(*approvalv1.ApprovalRequest)

4. Migrator delegates to Handler
   └─> handler.ComputeLegacyIdentifier(ctx, approvalRequest)

5. Handler executes business logic
   ├─> Check strategy (skip if Auto)
   ├─> Parse owner references
   ├─> Compute legacy name (with component swap)
   └─> Compute legacy namespace (strip environment)

6. Handler returns result
   └─> (namespace, name, skip, err)

7. Migrator returns to Controller
   └─> (namespace, name, skip, err)

8. Controller fetches from legacy cluster
   └─> FetchFromLegacy(ctx, remoteClient, namespace, name)
   
... (similar delegation for HasChanged, ApplyMigration)
```

## Benefits of This Architecture

### ✅ Plugin System Benefits
- **Extensibility:** Add new resource types without touching framework
- **Modularity:** Each migrator is independent
- **Dynamic Registration:** Enable/disable migrators at runtime
- **Reusable Framework:** One controller handles all types

### ✅ Handler Pattern Benefits
- **Separation of Concerns:** Orchestration vs business logic
- **Testability:** Handler methods easy to unit test
- **Type Safety:** Handler works with concrete types
- **Readability:** Business logic not mixed with interface boilerplate

### ✅ Combined Benefits
- **Best of Both Worlds:** Flexibility + Clean Code
- **Maintainability:** Clear responsibilities per component
- **Scalability:** Easy to add new migrators
- **Developer Experience:** Simpler to understand and modify

## Adding a New Migrator

To add a new resource type (e.g., `Rover`):

### 1. Create Handler (`internal/migrators/rover/handler.go`)

```go
type MigrationHandler struct {
    log logr.Logger
}

func (h *MigrationHandler) ComputeLegacyIdentifier(
    ctx context.Context,
    rover *roverv1.Rover,
) (namespace, name string, skip bool, err error) {
    // Business logic for Rover migration
}

// ... other handler methods
```

### 2. Create Migrator (`internal/migrators/rover/migrator.go`)

```go
type RoverMigrator struct {
    handler *MigrationHandler
}

func (m *RoverMigrator) ComputeLegacyIdentifier(
    ctx context.Context,
    obj client.Object,
) (namespace, name string, skip bool, err error) {
    rover := obj.(*roverv1.Rover)
    return m.handler.ComputeLegacyIdentifier(ctx, rover)
}

// Implement other interface methods...
```

### 3. Register in `cmd/main.go`

```go
// Register migrators
if err := registry.Register(approvalrequest.NewApprovalRequestMigrator()); err != nil {
    setupLog.Error(err, "failed to register ApprovalRequest migrator")
    os.Exit(1)
}

if err := registry.Register(rover.NewRoverMigrator()); err != nil {
    setupLog.Error(err, "failed to register Rover migrator")
    os.Exit(1)
}
```

**That's it!** No changes to framework code needed. ✨

## Comparison with Migration Module

| Aspect | Migration Module | Migrator Module |
|--------|-----------------|-----------------|
| **Pattern** | Controller + Handler | Plugin + Handler |
| **Extensibility** | New controller per type | Register new plugin |
| **Controller** | One per resource type | One generic controller |
| **Handler** | Direct business logic | Via plugin interface |
| **Flexibility** | Simple, straightforward | Highly extensible |
| **Best For** | Single resource type | Multiple resource types |

## Testing Strategy

### Unit Tests
- Test handlers directly (no mocking needed)
- Test mappers independently
- Mock remote client for handler tests

### Integration Tests
- Test migrator orchestration
- Test controller with fake clients
- Test end-to-end migration flow

### Example Handler Test

```go
func TestMigrationHandler_ComputeLegacyIdentifier(t *testing.T) {
    mapper := NewApprovalMapper()
    handler := NewMigrationHandler(mapper, ctrl.Log)
    
    approvalRequest := &approvalv1.ApprovalRequest{
        // ... test data
    }
    
    namespace, name, skip, err := handler.ComputeLegacyIdentifier(
        context.Background(),
        approvalRequest,
    )
    
    // Assertions...
}
```

**Clean and simple!** No complex mocking required. ✅

## Summary

The Migrator's **Plugin-based Handler Architecture** provides:

1. **Plugin System** for extensibility (add migrators dynamically)
2. **Handler Pattern** for clean, testable business logic
3. **Clear Separation** between orchestration and logic
4. **Best Developer Experience** for both framework and plugin developers

This architecture is production-ready, maintainable, and scales effortlessly! 🚀
