<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Projector

The projector is a read-only Kubernetes controller that watches Custom Resources and projects their state into a PostgreSQL database via the [ent](https://entgo.io/) ORM. It never writes back to the cluster — it only reads objects and persists them.

## CQRS Projector Pattern

This module implements the **projector** side of the CQRS (Command Query Responsibility Segregation) pattern. In CQRS, the write side (commands) and read side (queries) use separate models optimised for their respective workloads:

- **Command side** — Kubernetes Custom Resources are the source of truth; operators and users mutate them through the K8s API.
- **Query side** — The projector continuously watches those resources and materialises a relational read model in PostgreSQL, optimised for the GraphQL query layer.

The projector bridges these two worlds: it subscribes to change events on the command side and maintains a **materialised projection** — a denormalised, query-friendly representation of the same data. This is analogous to a materialised view in event-sourcing architectures, except the "event log" is the Kubernetes watch stream.

## Architecture

```
main.go                          26-line entrypoint → bootstrap.Run()
│
└─ internal/
   ├── bootstrap/                Wires DB, caches, manager; registers all modules
   ├── config/                   Env-var configuration
   ├── runtime/                  Generic pipeline: Translator → Processor → Reconciler
   ├── module/                   Type-erased Module interface + TypedModule[T,D,K]
   ├── infrastructure/           Shared components: DeleteCache, EdgeCache, IDResolver
   └── domain/
       ├── shared/               Common helpers (Metadata, StatusFromConditions)
       ├── zone/                 Level 0 — no FK dependencies
       ├── group/                Level 0
       ├── team/                 Level 1 — optional Group FK
       ├── application/          Level 2 — required Team + Zone FKs
       ├── apiexposure/          Level 3 — required Application FK
       ├── apisubscription/      Level 3 — required Application FK, optional target ApiExposure FK
       ├── approval/             Level 4 — required ApiSubscription FK
       └── approvalrequest/      Level 4 — required ApiSubscription FK
```

### Pipeline

Every module follows the same pipeline, built from three generic interfaces:

```
K8s Event
  │
  ▼
SyncEventHandler ──► captures last-known object in DeleteCache on delete
  │
  ▼
ReadOnlyReconciler ──► fetches the live object (or retrieves last-known on delete)
  │
  ▼
Processor[T, D, K]
  ├── ShouldSkip(obj)       → skip if object is not relevant
  ├── Translate(obj) → DTO  → map K8s object to domain payload
  └── Upsert(dto) / Delete(key) → persist via Repository
```

The core interfaces (`runtime/contracts.go`):

| Interface | Type Parameters | Responsibility |
|-----------|----------------|----------------|
| `Translator[T, D, K]` | T = K8s object, D = DTO, K = identity key | Maps objects to domain payloads and derives delete keys |
| `Repository[K, D]` | K = identity key, D = DTO | Typed persistence (Upsert / Delete) |
| `SyncProcessor[T]` | T = K8s object | Type-erased facade consumed by the reconciler |

### Infrastructure

| Component | Purpose |
|-----------|---------|
| `DeleteCache` | `sync.Map` that stores last-known objects on delete events |
| `SyncEventHandler` | Intercepts delete events to populate the DeleteCache |
| `EdgeCache` | Ristretto-based cache for foreign key IDs |
| `IDResolver` | Cache-first, DB-fallback FK lookups (satisfies each module's dependency interface) |

### Entity Dependency Hierarchy

```
Level 0:  Zone    Group
Level 1:         Team ─────────────┐
Level 2:  Application ◄────────────┘ (Team FK + Zone FK)
Level 3:  ApiExposure   ApiSubscription ◄── Application FK
Level 4:  Approval   ApprovalRequest ◄── ApiSubscription FK
```

## Adding a New Entity

Each entity lives in its own package under `domain/` with 4–5 files:

| File | Purpose |
|------|---------|
| `types.go` | DTO struct + identity key type |
| `translator.go` | Implements `Translator[T, D, K]` — maps K8s object to DTO |
| `repository.go` | Implements `Repository[K, D]` — ent upsert/delete logic |
| `deps.go` | Narrow dependency interface for FK resolution (ISP) |
| `module.go` | Exports a `Module` variable (single `TypedModule` struct literal) |

Then add one line to the module registry in `bootstrap/bootstrap.go`:

```go
var modules = []module.Module{
    // ...existing modules...
    yournewentity.Module,
}
```

## Error Handling

The runtime defines sentinel errors that control reconciler behavior:

| Error | Meaning | Reconciler Action |
|-------|---------|-------------------|
| `ErrSkipSync` | Object should not be synced | No requeue |
| `ErrDependencyMissing` | FK parent not yet synced | Requeue with backoff |
| `ErrDeleteKeyLost` | Cannot derive identity from delete event | Log warning, no requeue |
