# Plan: Add GraphQL Mutations for Team CRDs

## Context

The controlplane-api is currently read-only. We need to add `createTeam` and `updateTeam` GraphQL mutations that write directly to Kubernetes Team CRDs (not PostgreSQL). The existing DB Sync operator will reconcile CRD changes back to the DB asynchronously.

Write path: `GraphQL mutation → K8s API → Team CRD → (operator reconciles) → PostgreSQL`

## K8s Client Pattern — Matching the Domain Operators

The domain operators (organization, admin, identity, gateway) access the K8s API through a client hierarchy defined in `common/pkg/client/`:

```
controller-runtime client.Client  (base K8s API)
  → ScopedClient                   (auto-injects environment label, filters by environment)
    → JanitorClient                (tracks object state for orphan cleanup — operators only)
```

**Operator flow** (from `common/pkg/controller/controller.go`):
1. Base client comes from `mgr.GetClient()`
2. Wrapped per-reconciliation: `cc.NewJanitorClient(cc.NewScopedClient(c.Client, env))`
3. Injected into context, extracted by handlers, write via `CreateOrUpdate(ctx, obj, mutateFn)`

**Our adaptation for the controlplane-api:**
- Base `client.Client` created at startup via `client.New(restConfig, client.Options{Scheme: scheme})`, stored on the `Resolver` struct
- Per-mutation: wrap in `ScopedClient` with the environment from the mutation input
- **Skip JanitorClient** — it exists for orphan cleanup during operator reconciliation cycles, which doesn't apply to single mutation calls
- **Skip context-based injection** — the resolver already has the client via its struct field; no indirection layer needs it passed through context
- Use the same `ScopedClient.CreateOrUpdate(ctx, obj, mutateFn)` pattern as operators for the actual K8s write
- `ScopedClient` automatically handles environment label injection and namespace defaulting

Key difference from operators: we derive environment from the GraphQL mutation input (not from reconciled object labels), and we use `ScopedClient` directly without `JanitorClient` wrapping.

## K8s Namespace Convention

From `organization/api/v1/team_types.go:FindTeamForNamespace`:
- **Namespace**: `<environment>` (e.g., `dev`)
- **Resource name**: `<group>--<team>` (e.g., `group-a--team-alpha`)

## Authorization Model

| Viewer Type | createTeam | updateTeam |
|-------------|-----------|-----------|
| Admin | Any team | Any team |
| Group | Teams within their group | Teams within their group |
| Team | Not allowed | Only their own team |

## Architecture

```
GraphQL Mutation Request
  │
  ├─ Fiber (common-server) → JWT + BusinessContext
  ├─ gqlgen handler
  │   └─ ViewerFromBusinessContext → Viewer (with Group field)
  ├─ Mutation Resolver (thin — delegates to service)
  │   └─ TeamService interface
  │       └─ K8s implementation
  │           ├─ Authorization check (viewer-based)
  │           ├─ Input validation delegated to K8s (webhook/operator)
  │           ├─ Create ScopedClient for the request's environment
  │           ├─ Build K8s Team CRD object
  │           └─ scopedClient.CreateOrUpdate(ctx, obj, mutateFn)
  └─ Return TeamMutationResult (acknowledgment, not DB entity)
```

The resolver does not know about Kubernetes. It depends on a `TeamService` interface, which has a K8s-backed implementation. This separation:
- Makes resolvers trivially testable (mock the interface)
- Allows swapping the backend without touching resolver code
- Keeps authorization, validation, and K8s logic in the service layer
- Scales to other entities — each gets its own service interface

The mutation path **bypasses ent entirely** — it writes to Kubernetes, not PostgreSQL. The existing `PrivacyMixin` with `AlwaysDenyRule` for mutations remains unchanged since it only applies to ent operations.

---

## Implementation Steps and Gates

### Phase 1: Foundation (Viewer + Config + Dependencies)

#### Step 1. Update go.mod — add dependencies

Add direct dependencies:
- `github.com/telekom/controlplane/organization/api` — Team CRD types
- `github.com/telekom/controlplane/common` — ScopedClient, config (EnvironmentLabelKey)

Add `replace` directives for local development.

#### Step 2. Extend Viewer with Group field

**`internal/viewer/viewer.go`** — Add `Group string` to `Viewer` struct

**`internal/graphql/viewer_middleware.go`** — Set `v.Group = bCtx.Group` in the `ClientTypeGroup` case

This preserves group identity for authorization without coupling resolvers to the security middleware.

#### Step 3. Add Kubernetes config

**`cmd/config/config.go`** — Add:
```go
type KubernetesConfig struct {
    Enabled    bool   `yaml:"enabled"`
    Kubeconfig string `yaml:"kubeconfig"` // optional, defaults to in-cluster
}
```
Add `Kubernetes KubernetesConfig` to `ServerConfig`. Default: `Enabled: false`.

> ### Gate 1: Foundation compiles and existing tests pass
> - [ ] `go build ./...` succeeds
> - [ ] `make test` passes — no regressions from Viewer or config changes
> - [ ] Existing `viewer_middleware_test.go` still passes (Group field is additive)

---

### Phase 2: GraphQL Schema + Code Generation

#### Step 4. GraphQL schema — new file `mutation.graphql`

```graphql
input CreateTeamInput {
  environment: String!
  group: String!
  name: String!
  email: String!
  members: [MemberInput!]!
  category: TeamCategoryInput! = CUSTOMER
}

input UpdateTeamInput {
  environment: String!
  group: String!
  name: String!
  email: String
  members: [MemberInput!]
  category: TeamCategoryInput
}

input MemberInput {
  name: String!
  email: String!
}

enum TeamCategoryInput { CUSTOMER, INFRASTRUCTURE }

type TeamMutationResult {
  success: Boolean!
  message: String!
  namespace: String
  resourceName: String
}

type Mutation {
  createTeam(input: CreateTeamInput!): TeamMutationResult!
  updateTeam(input: UpdateTeamInput!): TeamMutationResult!
}
```

#### Step 5. Add Go model types — new file `internal/resolvers/model/mutation_types.go`

Define `TeamMutationResult`, `CreateTeamInput`, `UpdateTeamInput`, `MemberInput`, `TeamCategoryInput` Go structs.

#### Step 6. Update gqlgen.yml

Add model mappings for `TeamMutationResult`, `CreateTeamInput`, `UpdateTeamInput`, `MemberInput`, `TeamCategoryInput` pointing to `internal/resolvers/model`.

#### Step 7. Define TeamService interface and update Resolver

**New file `internal/service/team.go`** — interface definition:
```go
// TeamService defines operations for managing Team resources.
type TeamService interface {
    CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error)
    UpdateTeam(ctx context.Context, input model.UpdateTeamInput) (*model.TeamMutationResult, error)
}
```

**`internal/resolvers/resolver.go`** — resolver depends on the interface, not on K8s:
```go
type Resolver struct {
    client      *ent.Client
    teamService service.TeamService // nil if K8s disabled
}

func NewResolver(entClient *ent.Client, teamService service.TeamService) *Resolver
func NewSchema(entClient *ent.Client, teamService service.TeamService) graphql.ExecutableSchema
```

The resolver is thin — mutation methods just delegate to the service.

#### Step 8. Run `make generate`

Produces `mutation.generated.go` with resolver interface stubs.

> ### Gate 2: Code generation succeeds and schema is valid
> - [ ] `make generate` completes without errors
> - [ ] `mutation.generated.go` is created with `CreateTeam` and `UpdateTeam` resolver stubs
> - [ ] No type conflicts between manually defined models and gqlgen-generated code
> - [ ] Custom imports in existing `*.resolvers.go` files are intact (especially `viewer` package)

---

### Phase 3: Service + Mutation Implementation

#### Step 9. Implement K8s-backed TeamService — new file `internal/service/team_k8s.go`

```go
type teamK8sService struct {
    client client.Client // base controller-runtime client
}

func NewTeamK8sService(client client.Client) TeamService {
    return &teamK8sService{client: client}
}
```

Contains all K8s logic:
- `CreateTeam`: authorize (admin or matching group) → create `ScopedClient` for environment → build CRD → `scopedClient.CreateOrUpdate(ctx, obj, mutateFn)` → map result
- `UpdateTeam`: authorize (admin, matching group, or matching team) → create `ScopedClient` → `scopedClient.CreateOrUpdate(ctx, obj, mutateFn)` with partial updates → map result

Helper functions (private, in same file or `internal/service/helpers.go`):
- `authorizeCreateTeam(ctx, targetGroup)` — admin or matching group
- `authorizeUpdateTeam(ctx, targetGroup, targetTeam)` — admin, matching group, or matching team
- `teamResourceName(group, team)` — returns `<group>--<team>`
- `mapK8sError(err)` — maps K8s API errors to GraphQL errors with codes: `NOT_FOUND`, `ALREADY_EXISTS`, `CONFLICT`, `FORBIDDEN`, `VALIDATION_ERROR`, `INTERNAL`

Input validation (regex patterns, required fields, etc.) is handled by K8s admission webhooks and the operator — not duplicated here. Validation errors from K8s are surfaced via `mapK8sError`.

`CreateTeam` flow inside the service:
```go
func (s *teamK8sService) CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error) {
    // 1. Authorize (viewer from context)
    // 2. Create ScopedClient for environment
    scopedClient := cc.NewScopedClient(s.client, input.Environment)
    // 4. Build Team CRD
    team := &organizationv1.Team{ObjectMeta: metav1.ObjectMeta{
        Name:      teamResourceName(input.Group, input.Name),
        Namespace: input.Environment,
    }}
    // 5. CreateOrUpdate with mutate function (same pattern as operator handlers)
    _, err := scopedClient.CreateOrUpdate(ctx, team, func() error {
        team.Spec = organizationv1.TeamSpec{...}
        return nil
    })
    // 6. Map result/error to TeamMutationResult
}
```

#### Step 10. Implement mutation resolvers — new file `internal/resolvers/mutation.resolvers.go`

Resolvers are thin — they just delegate to the service:
```go
func (r *mutationResolver) CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error) {
    return r.teamService.CreateTeam(ctx, input)
}

func (r *mutationResolver) UpdateTeam(ctx context.Context, input model.UpdateTeamInput) (*model.TeamMutationResult, error) {
    return r.teamService.UpdateTeam(ctx, input)
}
```

> ### Gate 3: Mutations compile with service layer
> - [ ] `go build ./...` succeeds
> - [ ] No unimplemented interface errors from gqlgen-generated code
> - [ ] Service interface, K8s implementation, authorization, and error mapping all compile cleanly
> - [ ] Resolver methods delegate to service — no K8s imports in resolver package

---

### Phase 4: Wiring (main.go)

#### Step 11. Initialize K8s client and service in main.go

**`cmd/main.go`**:
- Register scheme: `clientgoscheme.AddToScheme` + `organizationv1.AddToScheme`
- If `cfg.Kubernetes.Enabled`: create base client via `client.New(restConfig, client.Options{Scheme: scheme})`
  - `restConfig` from `ctrl.GetConfigOrDie()` or `clientcmd.BuildConfigFromFlags("", cfg.Kubernetes.Kubeconfig)` if kubeconfig path provided
  - Create service: `teamService := service.NewTeamK8sService(k8sClient)`
- Pass service to `NewSchema(entClient, teamService)`

> ### Gate 4: Full build succeeds
> - [ ] `make build` produces `bin/controlplane-api` without errors
> - [ ] Binary starts with default config (K8s disabled) — no panic, no startup errors
> - [ ] Existing query functionality is unaffected (GraphQL playground still works when K8s is disabled)

---

### Phase 5: Tests

#### Step 12. Write tests

**New file `internal/service/team_k8s_test.go`** — tests the K8s service implementation:
Using Ginkgo/Gomega + `controller-runtime/pkg/client/fake`:
- Authorization tests: admin, group (matching/non-matching), team (own/other), no viewer
- createTeam: success, already exists, K8s validation rejection
- updateTeam: success, not found, conflict, partial update
- Error mapping tests
- Verify that ScopedClient correctly injects environment labels on created objects

**New file `internal/resolvers/mutation_resolvers_test.go`** — tests resolver delegation:
- Verifies resolvers correctly delegate to service interface
- Uses a mock `TeamService` (no fake K8s client needed at resolver level)

> ### Gate 5: All tests pass and code is clean
> - [ ] `make test` passes — all existing tests + new service and resolver tests
> - [ ] `make lint` passes — no linting violations
> - [ ] Test coverage for authorization: all viewer types tested for both mutations
> - [ ] Test coverage for K8s errors: at least NOT_FOUND, ALREADY_EXISTS, and CONFLICT mapped correctly

---

## File Summary

| File | Phase | Action |
|------|-------|--------|
| `go.mod` | 1 | Add organization/api + common dependencies |
| `internal/viewer/viewer.go` | 1 | Add Group field to Viewer |
| `internal/graphql/viewer_middleware.go` | 1 | Populate Group from BusinessContext |
| `cmd/config/config.go` | 1 | Add KubernetesConfig |
| `mutation.graphql` | 2 | **NEW** — mutation schema |
| `internal/resolvers/model/mutation_types.go` | 2 | **NEW** — Go types for mutation I/O |
| `gqlgen.yml` | 2 | Add mutation model mappings |
| `internal/service/team.go` | 2 | **NEW** — TeamService interface |
| `internal/resolvers/resolver.go` | 2 | Add teamService field, update constructors |
| `internal/service/team_k8s.go` | 3 | **NEW** — K8s-backed TeamService implementation |
| `internal/service/helpers.go` | 3 | **NEW** — auth, error mapping |
| `internal/resolvers/mutation.resolvers.go` | 3 | **NEW** — thin resolver delegation to service |
| `cmd/main.go` | 4 | Init K8s scheme + client + service, pass to resolvers |
| `internal/service/team_k8s_test.go` | 5 | **NEW** — service implementation tests |
| `internal/resolvers/mutation_resolvers_test.go` | 5 | **NEW** — resolver delegation tests |

## Extensibility

This pattern is designed for reuse when adding mutations for other entities:
- Authorization helpers are parameterized (group, team) — adaptable to other entity ownership models
- K8s error mapping is generic
- `cc.NewScopedClient()` is reusable in any service implementation — just pass the environment
- Each entity gets a service interface + K8s implementation, following the same pattern
- New mutations follow the same flow: add `.graphql` types → model types → service interface → K8s implementation → `make generate` → thin resolver delegation
- `TeamMutationResult` can be generalized to `MutationResult` if other entities share the same response shape
