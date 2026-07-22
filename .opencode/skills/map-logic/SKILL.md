---
name: map-logic
description: Maps one gateway feature's current Kong/Jumper logic to Envoy xDS, climbing a default-first ladder (Envoy default > standard filter > ext_proc/Lua > new-Jumper sidecar) and recording the chosen rung and why higher rungs failed. Use when migrating a gateway feature to Envoy, or asked "how does feature X map to xDS", "map this feature to envoy", or building the Kong-to-Envoy mapping table.
---

# Map Logic

You map **one gateway feature at a time** from its current Kong/Jumper
implementation to an **Envoy xDS** construct. You produce a mapping row: the
feature, the xDS construct it lands on, the **ladder rung** it settled at, and
**why the higher (more native) rungs did not work**.

You do not decide feasibility alone and you do not cite Envoy docs yourself.
You orchestrate two others:
- **`gateway-expert`** (skill, same session) — supplies current behavior, cited
  to `gateway/**`.
- **`envoy-expert`** (subagent, Task tool) — supplies whether Envoy covers it,
  cited to Envoy docs.

## The ladder (the whole point)

For every feature, climb from the top and **stop at the first rung that
holds**. Higher = more native, less custom code, less to maintain.

```
1. Envoy default / built-in     → no filter config, native behavior covers it
2. Standard HTTP filter         → jwt_authn, rate_limit, ext_authz, rbac, cors, ...
3. ext_proc / Lua / Wasm        → custom logic in the proxy, no separate sidecar
4. new-Jumper sidecar           → LAST RESORT: behavior has no in-proxy analog
```

**Bias, non-negotiable:** prefer Envoy defaults over custom logic in the
sidecar. Ask `envoy-expert` **default-first** — start the question at rung 1,
only descend when it answers NOT SUPPORTED. new-Jumper (rung 4) is reached
**only** when rungs 1-3 are exhausted, and the mapping row must justify it.

The "new-Jumper" is the future Envoy-based replacement for the Jumper sidecar;
even for it, prefer Envoy defaults over reimplementing custom Jumper logic.

## Procedure (one feature)

1. **Current logic** — ask `gateway-expert`: what does this feature do today?
   Get the feature file, the `IsUsed`/`Apply` behavior, and what it produces
   (Kong plugin or `JumperConfig`/`RoutingConfigs` field), all cited.
   Reduce it to a **behavioral requirement** — what must be true for a
   request, independent of Kong. (E.g. AccessControl = "reject tokens whose
   issuer isn't trusted AND whose consumer isn't in the allow-list.")

2. **Climb the ladder** — take the behavioral requirement to `envoy-expert`,
   phrased default-first:
   > "Does Envoy cover <behavior> natively? If not, is there a standard HTTP
   > filter? If not, ext_proc/Lua/Wasm?"
   Take its cited answer (SUPPORTED / PARTIAL / NOT SUPPORTED + mechanism).
   Descend only as far as needed.

3. **Emit the mapping row:**
   ```
   | Feature | Current (cited) | xDS construct | Rung | Why not higher | Citation |
   ```
   - PARTIAL → note the caveat and what closes the gap.
   - Rung 4 (new-Jumper) → the "Why not higher" cell must show rungs 1-3 were
     each ruled out (with the envoy-expert citation for each NOT SUPPORTED).

## Feature reference (from gateway-expert)

The 14 features and what they produce today live in the `gateway-expert` skill.
Ask it rather than re-deriving. The known-hard ones — LastMileSecurity,
ExternalIDP, CustomScopes, Claims, LoadBalancing, Failover — go through
`JumperConfig`/`RoutingConfigs` (the sidecar), so they are the most likely to
descend the ladder. Map the **easy independent** features first (AccessControl,
RateLimit, IpRestriction, HeaderTransformation) to validate the approach.

## Rules

- **One feature per pass.** Batch only by fanning out independent features in
  parallel Task calls — never merge two features into one row.
- **No self-sourced Envoy claims.** Every "Envoy can/can't" comes from
  `envoy-expert` with a citation. No citation → "unverified", descend or stop.
- **No self-sourced operator claims.** Every "today it does X" comes from
  `gateway-expert` with a `path:line` citation.
- **Justify every descent.** A lower rung without a recorded reason the higher
  rung failed is an incomplete row.
- You produce the **mapping**, not verdicts against the spec — that's
  `evaluate-envoy`. Hand your table off to it for the requirements gate.

## Output

The mapping table (one row per feature) and nothing else, unless code is
explicitly requested. End with a one-line rung tally
(e.g. "3 default, 4 filter, 2 ext_proc, 5 new-Jumper").
