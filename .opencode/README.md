<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Gateway: Kong → Envoy xDS Migration Workflow

Tooling to rewrite the gateway-operator's **FeatureBuilder** from **Kong/Jumper**
config to **Envoy xDS**. Guiding rule: **prefer Envoy defaults over custom
sidecar logic.**

## Actors

| Actor | Type | Knows | Job |
|---|---|---|---|
| `gateway-expert` | skill | our operator (cites `gateway/**`) | Explains current behavior, drafts code, orchestrates |
| `map-logic` | skill | Kong/Jumper → xDS mapping | Maps one feature at a time via the default-first ladder |
| `envoy-expert` | subagent | Envoy product (cites docs) | Answers "can Envoy do this natively?" |
| `evaluate-envoy` | skill | `requirements.md` | Gates the mapping: MET / NOT-MET verdicts |
| `implement-envoy` | skill | `EnvoyProxyFeatureBuilder` (`gateway/**`) | Builds one MET-gated feature into the Envoy xDS builder |

Only `gateway-expert` cites our code; only `envoy-expert` cites Envoy docs.

## The ladder (map-logic core)

Per feature, stop at the first rung that holds:

1. Envoy default / built-in
2. Standard HTTP filter (jwt_authn, rate_limit, ext_authz, rbac, …)
3. ext_proc / Lua / Wasm (in-proxy custom logic)
4. new-Jumper sidecar — **last resort**, must justify why 1–3 failed

## Workflow

1. **Map** — `map-logic` per feature: `gateway-expert` gives current behavior,
   `envoy-expert` (default-first) gives the Envoy mechanism → mapping table row
   (feature → xDS construct + rung + why not higher).
2. **Gate** — `evaluate-envoy` checks the table against
   `.opencode/agent/strict-reviewer/requirements.md`. NOT-MET rows loop back to
   step 1.
3. **Build** — `implement-envoy` per MET-gated feature: it takes the mapping
   row as spec, gets current behavior from `gateway-expert` + proto shapes from
   `envoy-expert`, and emits the feature into the `EnvoyProxyFeatureBuilder`
   (parallel go-control-plane xDS builder in `gateway/internal/features/envoy/`,
   selected by `--feature-builder=envoy`, Kong default). Easy independent
   features first (AccessControl, RateLimit, IpRestriction); Jumper-derived
   features (last-mile, OAuth, claims, failover) last.
4. **Verify** — `make build` / `make test` (Ginkgo/envtest), then
   `adversarial-review` or `review-pr` before merge.

## Start

Map the first feature (suggested: AccessControl) with `map-logic`.
