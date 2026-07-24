<!--
Copyright 2026 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Envoy Feature Builder

Tracks how the Envoy `FeatureBuilder` replaces the Kong-based builder. Each Kong
feature emits Kong plugin / Jumper config today; the Envoy builder must reproduce
the same behaviour via Envoy xDS.

**Current state:** `builder.go` is a stub (`Build`/`BuildForConsumer` return
`BlockedErrorf("... not implemented yet")`) and `feature/access_control.go`
panics. Every feature below is **Planned**.

## Feature Support Matrix

Legend: **Done** = implemented & wired · **WIP** = in progress · **Planned** = not started.

### Independent Features

| Feature | Kong Priority | Purpose | Envoy |
|---|---|---|---|
| PassThrough | 0 | Route straight to upstream(s), no last-mile security (`route.Spec.PassThrough`). | Planned |
| AccessControl | 10 | ACL: restrict access by the route's trusted issuers. | Planned |
| RateLimit | 10 | Limit request rate per route/consumer (Redis-backed). | Planned |
| HeaderTransformation | 0 | Add/modify request & response headers on the primary route. | Planned |
| BasicAuth | 10 | HTTP Basic auth for consumers. | Planned |
| IpRestriction | 10 | Allow/deny IPs or CIDRs, per consumer. | Planned |
| CircuitBreaker | 110 | Trip traffic away from failing upstreams; rewrites service host (highest priority). | Planned |
| DynamicUpstream | LMS+1 (101) | Set `remote_api_url` dynamically; overrides upstream resolution. | Planned |

### Dependent Features

| Feature | Kong Priority | Depends On | Purpose | Envoy |
|---|---|---|---|---|
| LastMileSecurity | 100 | AccessControl | Inject/validate last-mile JWT (Jumper) gateway↔upstream. | Planned |
| ExternalIDP | CustomScopes-1 (98) | LastMileSecurity | Exchange tokens against an external IDP. | Planned |
| CustomScopes | LMS-1 (99) | LastMileSecurity | Attach custom OAuth scopes to the outgoing token. | Planned |
| Claims | 10 | LastMileSecurity | Write provider exposure token claims into JumperConfig. | Planned |
| LoadBalancing | LMS+2 (102) | LastMileSecurity | Distribute across multiple upstreams (>1). | Planned |
| Failover | CircuitBreaker-1 (109) | LastMileSecurity | Route to a secondary upstream when the primary is down. | Planned |

### Constraints

- **ExternalIDP and LastMileSecurity are mutually exclusive** in the new builder
  (differs from Kong, where ExternalIDP depends on LastMileSecurity).
- Today Kong's LastMileSecurity forwards to the Jumper sidecar, which owns all
  other token/upstream features via `remote_api_url`. Jumper will be **removed**
  or **split into narrow-scope components** — not one monolithic sidecar.
- Kong features implement `features.KongFeature`; Envoy features implement
  `features.EnvoyFeature` (`internal/features/interfaces.go`).
- Priorities determine apply order; documented so the Envoy builder can preserve
  equivalent ordering.

## Domain ↔ xDS Vocabulary

`A == B` means "our A is Envoy's B". xDS types are v3 API types.

| Domain concept | Envoy xDS resource / field | xDS API type |
|---|---|---|
| Gateway | The xDS node fed via ADS (LDS/RDS/CDS/EDS). Not a config object. | `config.bootstrap.v3.Bootstrap` (node) |
| Route (CRD) | A `VirtualHost` with N `Route` entries — **not** a single Envoy `Route`. | `route.v3.VirtualHost` + `route.v3.Route` |
| Route.Hostnames | `VirtualHost.domains` (empty = `["*"]`). | `route.v3.VirtualHost.domains` |
| Route.Paths | `Route.match`, one Envoy `Route` per path (empty = `prefix: "/"`). | `route.v3.RouteMatch` |
| Backend.Upstreams (>1) | `RouteAction.weighted_clusters` → one `ClusterWeight` per upstream. | `route.v3.WeightedCluster.ClusterWeight` |
| Backend.Upstreams (single) | `RouteAction.cluster`. | `route.v3.RouteAction.cluster` |
| Upstream (single target) | An `LbEndpoint` in the Cluster's `ClusterLoadAssignment`. | `endpoint.v3.ClusterLoadAssignment` / `LbEndpoint` |
| Upstream weight | `ClusterWeight.weight` or `LbEndpoint.load_balancing_weight`. | as above |
| Route.Type primary/secondary | Endpoint priority (P0/P1) in one CLA, or an aggregate cluster. | `LocalityLbEndpoints.priority`; `envoy.clusters.aggregate` |
| Route.Type proxy | A `Cluster` whose endpoints are other Envoys (mesh hop). No dedicated type. | `cluster.v3.Cluster` |
| PassThrough | Disable auth filters via `typed_per_filter_config`. | per-route `typed_per_filter_config` |
| Security.TrustedIssuers | One `JwtProvider` per issuer, referenced by `requires`. | `jwt_authn` — `JwtProvider.issuer` |
| Consumer | Authenticated principal (JWT metadata / RBAC). No resource. | `jwt_authn` + `rbac` |
| ConsumeRoute | Per-route RBAC policy matching the consumer's principal. | `rbac` — `Policy` / `Principal` |
| ConsumeRoute rate limit | Rate-limit descriptor keyed on consumer identity; external RLS. | `ratelimit` — `route.v3.RateLimit` |
| Consumer IP restriction | RBAC `remote_ip` / `direct_remote_ip` CIDR. | `rbac` — `Principal.remote_ip` |
| Transformation | Route/vhost header add/remove, or Header Mutation filter. | `route.v3.Route` header fields; `header_mutation` |
| Buffering | Buffer filter (request); route `request_body_buffer_limit`. Response buffering not first-class. | `buffer` |

**Not 1:1 — watch out:**

- **"Route" ≠ Envoy `Route`** — ours = a `VirtualHost` with N `Route` entries. Three xDS levels.
- **primary/secondary** — endpoint priority (P0/P1, recommended, health-driven) vs. aggregate cluster (when cluster-level config differs). Decision open.
- **`proxy`** — no Envoy equivalent; just a `Cluster` pointing at another Envoy.
- **Consumer** — no registry; create/delete mutates JWT providers + RBAC principals on affected routes. Biggest gap.
- **ConsumeRoute** fragments across RBAC + ratelimit descriptor + per-filter M2M config. No single xDS object.
- **Response-body buffering** — not a first-class toggle; likely ext_proc/Lua.

## Builder Structure

The `EnvoyFeatureBuilder` reuses Kong's **execution model** (the `Feature`
contract `Name/Priority/IsUsed/Apply` + sorted, sequential, fail-fast apply loop)
but not its state shape.

**Keep from Kong:**
- Enable → ordered apply → assemble three-phase flow.
- `SortFeatures` by `Priority()` ascending, then `if IsUsed { Apply }`, sequential, fail-fast (requeue on first error).
- Shared feature-mutated state in the builder so later features see earlier writes.

**Drop / change:**
- **No `map[string]cachedProto` with panic getters** — use typed draft state; wrong types = compile errors, invariant violations = `error`, never panic.
- **No per-plugin create/replace/cleanup** — Envoy pushes one bundle.
- **Narrow mutation surface** — small helpers (`EnsureVirtualHost`, `EnsureCluster`, `AddJWTProvider`, `AddRBACPolicy`, `AddRateLimitDescriptor`), not raw proto access.
- **Deterministic ordering** — sort map keys before emitting slices.

**Priority vs. filter-chain order:** `Priority()` orders feature computation /
data dependencies only. Envoy's HTTP filter order (jwt_authn → rbac → ratelimit)
is protocol-semantic and fixed by a **canonical order table in the assemble
phase**, never derived from priority.

## Production Architecture

The unit of correctness is the **node snapshot** (all resources consistent
together), not a per-route push. `XdsClient.SetSnapshotFor(ctx, nodeID, bundle)`
replaces the whole node bundle.

**Topology (scale-defining):** multiple CP-Gateways, each pinned to a region;
each == one Envoy Deployment scaled to **100+ identical pods** (HPA). One control
plane per region serves all pods via xDS/ADS.

**One snapshot per Gateway, keyed by Gateway identity:**
- go-control-plane's `SnapshotCache` keys on `NodeHash.ID(node)`, not the stream.
- **Do NOT use default `IDHash`** (per-pod `node.id`) → 100+ duplicate snapshots.
- **Use a custom `NodeHash`** keying on `node.metadata.role` (e.g. `region~gateway`) → one snapshot, one `SetSnapshot` fans out to all pods.
- Recompute is **O(1) in pod count**; only fan-out/serving is O(pods).
- Zone-aware LB needs no per-pod EDS — Envoy runs a locality heuristic against the shared CLA using each pod's `node.locality.zone` (set via Downward API).
- **Per-connected-client (kgateway UCC) not needed** — replicas are identical.

**Model (full-recompute-per-Gateway + hash-diff gate + shared node key)**, taken
from [kgateway](https://github.com/kgateway-dev/kgateway) — see [Designing
kgateway for scalability](https://kgateway.dev/blog/design-kgateway-for-scalability/):
- **NodeSnapshotBuilder** (per Gateway) recomputes all routes into one `ResourceBundle` on any change; runs each route's feature loop; canonicalizes filter order; validates referential integrity before publish.
- **Hash-diff gate** — push only if the content hash changed. Mandatory at 100+ streams (else all pods re-ACK).
- **Single writer per Gateway key**; **retain last-good** on incomplete input; **content-hash versions** for restart determinism.
- The per-Route `EnvoyFeatureBuilder` runs the feature loop but **does not push**; the node builder aggregates and owns `SetSnapshotFor`.
- `BuildForConsumer` is a **conceptual mismatch** (Consumer = RBAC principal / JWT identity) — keep only as a shim that marks routes stale and triggers recompute.

**CP HA & scale:**
- One CP deployment per region; each replica has its own in-memory `SnapshotCache`. No cross-replica coordination.
- xDS serving is **not** leader-gated; only K8s writes are (verified vs. istiod + kgateway).
- Multiple stateless replicas independently recompute the same deterministic snapshot. **No shared snapshot store, no sticky sessions** for correctness.
- **Readiness gating (required):** a fresh replica parks watches until its first `SetSnapshot`; gate readiness on "first snapshot for all served Gateways". Raise Envoy `init_fetch_timeout` (default 15s) if warm-up can exceed it.

**Two-tier leader election (controller-runtime):** we already have leader
election (`cmd/main.go:120`, `--leader-elect`) gating reconcilers. The xDS work
must run on **all** replicas — add a runnable with `NeedLeaderElection() == false`.

| Concern | Runs on | Why safe |
|---|---|---|
| Reconcile → status writes, finalizers, child CRs | Leader only | Single writer → no API-server races |
| Watch CRDs → build snapshot → serve xDS | All replicas | Read-only + idempotent; serves own connections |

The snapshot builder never writes to Kubernetes — it reads CRDs via the manager's
shared informer cache and serves an in-memory projection. It must be driven by
**informer events on the manager cache**, not the reconcile loop (which runs on
the leader only) — kgateway uses a collection layer
([krt](https://github.com/istio/istio/blob/master/pkg/kube/krt/README.md)) for this.

**Fan-out hazards at 100+ streams:**
- **ADS serial fan-out under lock** — every version bump is O(100) serial; budget & monitor.
- **Blocking response channels** — a stuck pod can back-pressure fan-out.
- **Thundering-herd re-ACK** — the concrete reason the hash-diff gate is non-negotiable.

**Operational rule:** never `ClearSnapshot` on pod disconnect (snapshot is
shared) — clear only when the whole Gateway is deleted.

> [!NOTE]
> Refuted: there is no `PILOT_ENABLE_XDS_LOAD_BALANCING` / connection-rebalancing
> knob in current Istio. Plan for connection stickiness to a replica for the
> stream's lifetime.

## Decision Checklist

**Decided (verified — implement as stated):**

1. **Snapshot granularity** — one per Gateway, shared by all pods; full recompute per Gateway on change.
2. **`NodeHash`** — key on `node.metadata.role` (e.g. `region~gateway`); never default `IDHash`.
3. **Versioning** — content hash of the assembled bundle; push only on change.
4. **CP HA / leader election** — reuse existing election for reconcilers; add xDS + snapshot builder as non-leader-gated runnable. No shared store, no sticky sessions.
5. **Trigger** — informer/`EventHandler` on the manager cache, not the reconcile loop.
6. **Failure policy** — retain last-good; never push partial.
7. **Readiness gating** — gate on "first snapshot for all served Gateways".
8. **Consumer path** — no `BuildForConsumer` pipeline; changes mark routes stale + recompute.
9. **Pod `node.locality.zone`** — wire via Downward API for zone-aware LB from the shared CLA.
10. **Operational** — never `ClearSnapshot` on pod disconnect; only on Gateway deletion.

**Open (needs a call):**

11. **Failover encoding** — endpoint priority (P0/P1) vs. aggregate cluster.
12. **Merge/recompute contract** — route enumeration, dedupe keys, conflict resolution.
13. **Canonical filter-order table** — fixed jwt_authn → rbac → ratelimit → … in assemble.
14. **Event coalescing** — debounce CRD bursts; single-writer serialization per Gateway.
15. **Consistency validation** — RDS↔CDS↔EDS referential integrity before publish.
16. **`init_fetch_timeout` vs. warm-up** — measure CP cold-start, tune if needed.
17. **Observability** — snapshot version, push count, suppressed pushes, validation failures, last-good age, stream count.
18. **Response-body buffering** — confirm feasibility (likely ext_proc/Lua).

## Per-Feature xDS Mapping

For each feature: what Kong/Jumper does today, the chosen Envoy xDS mapping, and
why. (To be filled in via `map-logic`.)

_PassThrough · AccessControl · HeaderTransformation · BasicAuth · IpRestriction ·
CircuitBreaker · DynamicUpstream · RateLimit · LastMileSecurity · ExternalIDP ·
CustomScopes · Claims · LoadBalancing · Failover — all TBD._
