---
name: implement-envoy
description: Implements one mapped gateway feature into the EnvoyProxyFeatureBuilder (go-control-plane xDS) in gateway/. Consumes a MET mapping-table row from map-logic/evaluate-envoy as the spec, drafts the xDS emission, and runs the build/test/review loop. Use after a feature has been mapped and gated, or when asked to "implement <feature> for envoy", "build the envoy feature builder", or "add <feature> to the EnvoyProxyFeatureBuilder".
permission:
  edit: allow
  bash: allow
  webfetch: allow
---

# Implement Envoy

You implement **one gateway feature at a time** into the
`EnvoyProxyFeatureBuilder` — the parallel, go-control-plane-based counterpart to
the Kong `FeaturesBuilder`. This skill is **step 3 (Build)** of the migration
workflow in `.opencode/README.md`; it consumes what `map-logic` (step 1) and
`evaluate-envoy` (step 2) produce.

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
- **Verify current behavior live.** Before drafting, re-read the feature file
  (`gateway/internal/features/feature/<feature>.go`) and note the exact spec
  fields its `Apply` reads. The Envoy path reads the **same source fields**; it
  does NOT run Kong `Apply`.
- **Kong path stays green.** Never touch the Kong `Builder`, the plugin types,
  the `FeaturesBuilder` interface, or the route handler. Envoy is additive,
  selected by the `--feature-builder=envoy` flag (Kong is the default).
- **Use Makefile targets** (AGENTS.md): `make build` / `make test` /
  `make lint` in `gateway/`, `make verify MODULES="gateway"` from repo root.
  Never call `go build`/`go test` directly.

## Target architecture (verify live — code drifts)

The first increment (AccessControl) established the shape; later features
follow it. This is now implemented in `gateway/internal/features/envoy/`.

- **Package:** `gateway/internal/features/envoy/`.
- **Composition over a shared base:** the neutral `features.FeatureBuilder`
  base (`gateway/internal/features/builder.go:46`) holds only backend-agnostic
  inputs (`GetRoute`, `GetConsumer`, `GetGateway`, `GetAllowedConsumers`,
  `AddAllowedConsumers`, `SetUpstream`). Kong's `FeaturesBuilder`
  (`builder.go:60`) and Envoy's `EnvoyFeatureBuilder` (`envoy/builder.go:40`)
  each **embed** it. There is NO Kong-plugin accessor panic-stub and NO
  `GetKongClient()` on the Envoy path — the two builders are separate interfaces
  sharing the base, not one interface with dead methods.
- **Feature type:** `EnvoyFeature` (`envoy/builder.go:25`) mirrors
  `features.Feature`, but `Apply` takes `EnvoyFeatureBuilder` and `IsUsed` takes
  the neutral `features.FeatureBuilder`. Compile-time enforced via
  `var _ EnvoyFeatureBuilder = &Builder{}` (`envoy/builder.go:55`).
- **Intent, not proto shapes:** feature `Apply` calls intent writers
  (`RequireJWT`, `AllowConsumers`) that accumulate on the builder; `Build`'s
  `render()` (`envoy/builder.go:194`) turns intent into xDS resources.
- **Write seam:** `XdsClient` (`envoy/client.go:41`), analog of
  `client.KongClient`, with `SetSnapshotFor(ctx, nodeID, ResourceBundle)`.
  Backed by a go-control-plane `SnapshotCache`: `cache.NewSnapshot` →
  `snap.Consistent()` → `cache.SetSnapshot` (`envoy/client.go:57`).
- **`Build(ctx)`** (`envoy/builder.go:154`) reads the feature source fields via
  `IsUsed`/`Apply`, renders one aggregated per-node snapshot, then
  `SetSnapshotFor`. POC uses a single hardcoded `PocNodeID`
  (`envoy/client.go:28`) — a flagged shortcut, not the end state.
- **Server:** in-operator xDS mgmt server (`server/v3` over gRPC ADS) added as
  a `manager.Runnable` in `gateway/cmd/main.go`, behind the flag.

## Per-feature procedure

1. **Confirm the gate.** Locate the feature's MET mapping row (rung + xDS
   construct). No MET row → stop, hand back to `evaluate-envoy`.
2. **Current behavior (`gateway-expert`).** Which spec fields does the Kong
   `Apply` read? What sentinels/edge cases (e.g. AccessControl's empty
   allow-list → `DenyAllGroup` deny-all)? Get `path:line` citations.
3. **Envoy shape (`envoy-expert`).** For the xDS construct in the row, get the
   fully-qualified v3 proto message, the 2-4 fields that matter, filter
   ordering, and edge-case encodings — each with a doc URL. Flag any config
   shape that depends on an unspecified decision and resolve it before coding.
4. **Draft** the emission: a pure `build<Feature>...` translator reading the
   same source fields (step 2), wiring the confirmed protos (step 3) into the
   `ResourceBundle`. Keep the snapshot internally consistent (routes reference
   real clusters). SPDX Apache-2.0 header; `logr` from context (V(0) for the
   snapshot publish, V(1) for detail); wrap errors with context.
5. **Test (Ginkgo v2/Gomega).** Unit-assert the emitted resources: unmarshal
   each filter `typed_config` into its concrete proto and assert fields; cover
   every edge case from step 2 (esp. the deny-all/sentinel path) and a real
   `SetSnapshot` + `Consistent()` round-trip. Run `make test` in `gateway/`.
6. **Review.** `make lint` + `make verify MODULES="gateway"`, then
   `adversarial-review` (or `review-pr`) before merge.

## Order of features

Easy independent first (AccessControl → RateLimit → IpRestriction), matching
`map-logic`. Consumer-scoped features (IpRestriction) also need the
`BuildForConsumer` path (`envoy/builder.go:189`, currently unimplemented).
Jumper-derived features (LastMile, OAuth, Claims, Failover) last. The shared
neutral-base extraction (`features.FeatureBuilder` embedded by both builders)
is already done — no Kong-plugin accessors on the Envoy path.

## Flagged POC shortcuts (carry into the PR description)

- Single hardcoded `PocNodeID` + whole-snapshot overwrite → single-route only;
  revisit with per-node IDs that accumulate a node's routes.
- Static STRICT_DNS cluster, dummy `local_jwks` inline JWKS → not
  production-shaped; revisit with EDS/TLS and real JWKS sources.

## Output

Code first, then at most a few lines: which feature, the rung it implemented,
the flagged shortcuts, and what to run to verify. End by pointing to the next
feature in the order.
