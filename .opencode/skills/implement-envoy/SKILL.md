---
# Copyright 2026 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: CC0-1.0

name: implement-envoy
description: Implements one mapped gateway feature into the Envoy feature builder (go-control-plane xDS) in gateway/internal/features/envoy/. Consumes a MET mapping-table row from map-logic/evaluate-envoy as the spec, drafts the xDS emission, and runs the build/test/review loop. Use after a feature has been mapped and gated, or when asked to "implement <feature> for envoy", "build the envoy feature builder", or "add <feature> to the envoy feature builder".
permission:
  edit: allow
  bash: allow
  webfetch: allow
---

# Implement Envoy

You implement **one gateway feature at a time** into the Envoy feature builder
(`gateway/internal/features/envoy/`) — the parallel, go-control-plane-based
counterpart to the Kong feature builder (`gateway/internal/features/kong/`).
This skill is **step 3 (Build)** of the migration workflow in `.opencode/README.md`;
it consumes what `map-logic` (step 1) and `evaluate-envoy` (step 2) produce.

You orchestrate existing actors — you do not re-derive their knowledge:
- **`gateway-expert`** (skill, same session) — exact current behavior of the
  feature, cited to `gateway/**`. Ask it what fields the Kong `Apply` reads.
- **`envoy-expert`** (subagent, Task tool) — the Envoy filter/proto shapes,
  cited to Envoy docs. Never assert Envoy behavior yourself.
- **`adversarial-review` / `review-pr`** (skills) — gate before merge.

## Hard rules

- **Only implement MET-gated features.** The mapping row (feature → xDS
  construct + rung) is your spec. No approved row → stop and route back to
  `map-logic`/`evaluate-envoy`. Do not implement un-mapped features.
- **No self-sourced Envoy claims.** Every proto shape / filter choice comes
  from `envoy-expert` with a doc-URL citation. No citation → ask, don't guess.
- **Verify current behavior live.** Before drafting, re-read the Kong feature
  file (`gateway/internal/features/kong/feature/<feature>.go`) and note the exact
  spec fields its `Apply` reads. The Envoy path reads the **same source fields**
  (Route/Consumer/Gateway spec); it does NOT run Kong `Apply`.
- **Kong path stays green.** Never touch the Kong `Builder`, the plugin types,
  the `KongFeatureBuilder` interface, or the Kong registration in the route
  handler. Envoy is additive, selected by `Gateway.Spec.GatewayClassName == "envoy"`
  (Kong is the default fallback).
- **Use Makefile targets** (AGENTS.md): `make build` / `make test` /
  `make lint` in `gateway/`, `make verify MODULES="gateway"` from repo root.
  Never call `go build`/`go test` directly.

## Target architecture (verify live — code drifts)

The package is split by backend. All shared contracts are in
`gateway/internal/features/interfaces.go`; each backend has its own subpackage.

```
gateway/internal/features/
  interfaces.go   # ALL interfaces (generic contract)
  util.go         # SortFeatures[T], ToSlice
  errors.go       # ErrNoRoute, ErrNoConsumer
  kong/           # kong.Builder + feature/ (all Kong feature impls, registered)
  envoy/
    builder.go    # envoy.Builder, NewEnvoyFeatureBuilder, Build, nodeIDForRoute
    routing.go    # renderCoreRouting + buildListener/buildRouteConfig/buildCluster
    xds.go        # ResourceBundle, XdsClient iface, XdsCache impl, hashResources
    nodehash.go   # nodeHash (snapshot keyed on node.metadata.role)
    server.go     # ADS gRPC mgmt server (manager.Runnable)
    feature/      # Envoy feature impls (currently AccessControl is a panic stub)
    *_test.go, README.md
```

### Interfaces (`interfaces.go`)

- **Neutral base** `FeatureBuilder` (`interfaces.go:16`): `GetRoute`, `GetConsumer`,
  `GetGateway`, `GetAllowedConsumers`, `AddAllowedConsumers`, `Build`,
  `BuildForConsumer`. **No `SetUpstream` here** — that is Kong-only.
- **Generic** `Feature[T FeatureBuilder]` (`interfaces.go:36`): `Name()`,
  `Priority()` (via `FeatureInfo`), `IsUsed(ctx, T)`, `Apply(ctx, T)`.
- `KongFeature = Feature[KongFeatureBuilder]` (`:48`); `KongFeatureBuilder`
  (`:50`) adds plugin accessors, `SetUpstream`, `GetKongClient()`.
- `EnvoyFeature = Feature[EnvoyFeatureBuilder]` (`:68`); `EnvoyFeatureBuilder`
  (`:70`) adds **only** `EnableFeature(EnvoyFeature)`. There are **no** intent
  writers (`RequireJWT`/`AllowConsumers`) and **no** `render()` — those do not exist.

### Envoy builder (`envoy/builder.go`)

- `Builder` implements `EnvoyFeatureBuilder` (`builder.go:17` compile-time assert).
- `NewEnvoyFeatureBuilder(xdsClient, route, consumer, gateway)` (`:31`) — a
  `var` func so tests can swap it.
- `Build(ctx)` (`:85`): require Route → sort features (`SortFeatures(ToSlice(...))`)
  → `IsUsed`/`Apply` loop (`:92`) → take `Upstreams[0]` (`:109`, single-upstream
  only) → `renderCoreRouting` → `SetSnapshotFor(nodeIDForRoute(route), bundle)`.
  **ponytail note (`:82`): feature `Apply` hooks currently run but do NOT mutate
  the bundle** — only core routing is emitted today. Wiring feature output into
  the bundle is part of implementing the first real feature (see below).
- `BuildForConsumer` (`:126`) is `BlockedErrorf` — not implemented.
- `nodeIDForRoute` (`:136`) = `route.Spec.GatewayRef.Name` (matches `nodeHash`
  on `node.metadata.role`). Per-route snapshot currently overwrites the whole
  Gateway snapshot (single-route-per-node ponytail shortcut).

### xDS assembly (`envoy/routing.go`)

Pure functions, no intent pipeline: `renderCoreRouting` (`:42`) →
`buildListener` (HCM + RDS-over-ADS + terminal router filter, `:61`),
`buildRouteConfig` (`:108`), `buildCluster` (STRICT_DNS, optional upstream TLS,
`:143`). `ResourceBundle` (`xds.go:28`) = `{Listeners, Clusters, Routes, Endpoints}`.
Canonical filter names are consts in `routing.go:31`.

### Write seam (`envoy/xds.go`)

`XdsClient.SetSnapshotFor(ctx, nodeID, ResourceBundle)` (`xds.go:36`) —
hash-diff gated (no-op if content unchanged). `XdsCache` (`:49`) wraps an ADS
`SnapshotCache`; `hashResources` (`:139`) gives content-addressed, restart-stable
versions. The ADS `Server` (`server.go`) serves from the same cache; it is a
`manager.Runnable`, always registered (not leader-gated).

### Backend selection & registration (`internal/handler/route/handler.go`)

`NewFeatureBuilder` (`handler.go:136`): if `gateway.Spec.GatewayClassName == "envoy"`
(`:146`) → `envoy.NewEnvoyFeatureBuilder(h.XdsClient, route, nil, gateway)` and
return. **The Envoy branch currently registers ZERO features** (`:147-149`); the
Kong fallback registers ~14 (`:161-173`). Constants: `GatewayClassNameEnvoy = "envoy"`
(`api/v1/gateway_types.go:29`). There is **no `--feature-builder` flag** — the only
xDS flag is `--xds-bind-address` in `cmd/main.go`.

### The one Envoy feature today is a panic stub

`envoy/feature/access_control.go` — `IsUsed`/`Apply` both `panic("unimplemented")`
(`:32,:37`), no `Instance...` registration var. Every feature is Planned per
`envoy/README.md`. So you are typically writing the **first real** Envoy feature.

## Per-feature procedure

1. **Confirm the gate.** Locate the feature's MET mapping row (rung + xDS
   construct). No MET row → stop, hand back to `evaluate-envoy`.
2. **Current behavior (`gateway-expert`).** Which spec fields does the Kong
   `Apply` read (now in `kong/feature/<feature>.go`)? What sentinels/edge cases
   (e.g. AccessControl's empty allow-list → deny-all)? Get `path:line` citations.
3. **Envoy shape (`envoy-expert`).** For the xDS construct in the row, get the
   fully-qualified v3 proto message, the 2-4 fields that matter, filter
   ordering, and edge-case encodings — each with a doc URL. Flag any config
   shape that depends on an unspecified decision and resolve it before coding.
4. **Decide the target seam:**
   - **Core-routing tweak** (path/host/cluster/timeout) → extend
     `renderCoreRouting`/`buildX` in `routing.go` and/or `ResourceBundle`.
   - **HTTP filter** (auth, rate-limit, transform) → the bundle has **no
     filter-emission seam yet**. The first filter feature must design it: a way
     for a feature's `Apply` to contribute HTTP filters (in canonical order)
     into `buildListener`'s `HttpFilters` (`routing.go:79`) before the terminal
     router. Keep it minimal (append-with-order), not a framework.
5. **Implement `envoy/feature/<feature>.go`:**
   - `var _ features.EnvoyFeature = &<Feature>Feature{}` compile-time assert.
   - `Name()` returns the **shared** `gatewayv1.FeatureType` constant (same one
     Kong uses); `Priority()` mirrors the Kong feature's priority.
   - `IsUsed`/`Apply` on `features.EnvoyFeatureBuilder`, reading the **same
     source spec fields** identified in step 2. Replace the panic stub.
   - Add an `Instance<Feature>Feature` package var (mirror the Kong
     `feature/` convention) so the handler can register it.
   - SPDX `Apache-2.0` header; `logr` from ctx (V(0) publish, V(1) detail);
     wrap errors with context; use `ctrlerrors.*` for reconciler-facing errors.
6. **Register it.** Add `builder.EnableFeature(feature.Instance<Feature>Feature)`
   to the **Envoy branch** of `route/handler.go` (currently empty, `:147-149`).
7. **Wire feature output into `Build`.** Make the `Apply` loop's contribution
   merge into `bundle` before `SetSnapshotFor` (the step-4 seam). Keep the
   snapshot internally consistent (routes reference real clusters).
8. **Test (Ginkgo v2/Gomega, `package envoy_test`).** Follow `builder_test.go`:
   build via `NewEnvoyFeatureBuilder` → `Build` → fetch snapshot with
   `XdsCache.Cache().(cachev3.SnapshotCache).GetSnapshot(gatewayName)` → unmarshal
   `typed_config` (see `unmarshalHCM`, `builder_test.go:165`) and assert fields.
   Cover every edge case from step 2 (esp. deny-all/sentinel) and a real
   round-trip. Run `make test` in `gateway/`.
9. **Review.** `make lint` + `make verify MODULES="gateway"`, then
   `adversarial-review` (or `review-pr`) before merge.

## Order of features

Easy independent first (AccessControl → RateLimit → IpRestriction), matching
`map-logic`. Consumer-scoped features (IpRestriction) also need the
`BuildForConsumer` path (`envoy/builder.go:126`, currently `BlockedErrorf`).
Jumper-derived features (LastMile, OAuth, Claims, Failover) last.

## Flagged POC shortcuts (carry into the PR description)

- `nodeIDForRoute` keys per-Route on the Gateway name, but each Route overwrites
  the Gateway's whole snapshot → single-route-per-node. Accumulate all routes of
  a Gateway into one bundle before this is production-shaped.
- Single-upstream only (`builder.go:109`); weighted clusters for multi-upstream
  is a later increment.
- Static STRICT_DNS cluster, inline endpoint, no EDS/TLS validation
  (`routing.go:143`) → not production-shaped.
- Fixed listen port `10000` (`routing.go:28`); Gateway CRD has no listen-port field.

## Output

Code first, then at most a few lines: which feature, the rung it implemented,
the flagged shortcuts, and what to run to verify. End by pointing to the next
feature in the order. For CP HA / scale / snapshot-cache design questions, defer
to `envoy/README.md` as the authoritative source rather than restating it here.
