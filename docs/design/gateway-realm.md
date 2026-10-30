<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Gateway Realm: realm-prefixed Kong entity names

Status: **Phase 1 and Phase 1b implemented** (prefix on the Gateway). Phase 2 (rollout) open.
This document matches the code; user-facing docs: `docs/docs/architecture/gateway.mdx` and `gateway-realm-migration.mdx`.

## Problem

The Gateway operator names every Kong entity it creates for a `Route` after `Route.metadata.name`. Several virtual
environments (e.g. `bond`, `cit2`, `cit4`) can share one Kong instance. The same Route name in two environments then
maps to the same Kong route, service and upstream, and the environments overwrite each other.

Agentic and API Routes do not collide with each other, because agentic names already start with `ai-gateway--`.
API and AI Routes on the same Gateway share one prefix.

## Decisions

| # | Decision |
|---|----------|
| D1 | Kong name: `<namePrefix>--<metadata.name>`, e.g. `bond--eni-echo-v3`, `default--eni-foo-v1`, `default--ai-gateway--mcp-v1`. The pattern is the same for every realm, including `default`. |
| D2 | The prefix is configured once per Gateway: optional field `Gateway.spec.namePrefix`, set by the admin operator to `zone.Status.RealmName`. All Routes referencing the Gateway use it; there is no per-Route field. `Route.spec.security.realmName` is **not** used, because it controls the Last-Mile-Security realm. API and AI Routes on the same Gateway share the prefix; agentic names (`ai-gateway--…`) keep them apart. |
| D3 | Gateway has a `namePrefix` → realm naming and migration for every Route on it. No prefix → Kong names, lookups and tags stay as they are today. The only exception is legacy plugin adoption (D11), which applies to all Routes. |
| D4 | Every Kong entity named after the route gets the new name: **route, service, upstream** (circuit breaker), and the service host that points to the upstream. Targets have no name. |
| D5 | Every Route on a Gateway with a prefix is migrated, including identity, zone-health and admin managed routes (no special handling). Only the admin operator changes; the api, agentic and event operators stay as they are. There is no feature flag: everything rolls out at once. |
| D6 | The tags `env--<env>` and `route--<metadata.name>` stay unchanged. A new tag `realm--<namePrefix>` is added. |
| D7 | Plugins are bound to their route by Kong **route ID**, not by route name. |
| D8 | Renaming happens in the regular full-body `PUT /{entity}/{id}`, together with all other changes. |
| D9 | Tags are only ever added on adopted entities. Existing `ei__telekom__de--*` tags are kept and removed in a later cleanup. |
| D10 | If, during migration, a route is found under one name and a **different** route we own exists under another candidate name → `BlockedError`. This should never happen, and nothing is deleted automatically. |
| D11 | Legacy plugins are adopted by the slot they occupy on the adopted route (same name, route ID and consumer), not by legacy tags. `LegacyPluginTag` and `legacyPluginTags` are removed. |
| D12 | Java naming rules live in an optional config file, mounted from a ConfigMap. The code reads it only through a `Migrator` interface. Without the file, no Java names are tried. |
| D13 | Names, IDs and the legacy origin are recorded in `Route.status.properties`. |
| D14 | Out of scope: Kong consumer name collisions, migrating infrastructure routes, and removing legacy tags. |
| D15 | `Gateway.spec.namePrefix` may change but must not be removed once set (CEL rule): without a prefix the Routes are looked up by `metadata.name` only, so second Kong routes would be created. The admin operator keeps an existing prefix if the realm name is ever empty. |
| D16 | Delete never blocks: foreign entities and duplicates are skipped, at most one entity per kind is deleted. |
| D17 | Precondition: the Java operator no longer writes a route once the CP has taken it over. The conflict check (D10) therefore runs only while a rename is pending; a Java route recreated later under its old name would not be detected. Accepted consequence: if a delete fails after removing the first of two duplicates (e.g. on the service delete or the finalizer update), the retry finds the retained duplicate and deletes it too. Duplicates exist only if the Java operator wrote after the takeover, which is excluded. |

## API change

`gateway/api/v1/gateway_types.go`, `GatewaySpec`. The field is optional, so the change is additive:

```go
// NamePrefix is prepended to the Kong entity names (route, service, upstream) of every Route on
// this Gateway as "<namePrefix>--<metadata.name>". It separates realms on a shared Kong.
// If empty, the Kong entities are named after metadata.name. Once set, it cannot be removed.
// +kubebuilder:validation:Optional
// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-?[a-z0-9]+)*$`
// +kubebuilder:validation:MaxLength=63
NamePrefix string `json:"namePrefix,omitempty"`
```

plus a CEL transition rule on the spec: a set `namePrefix` cannot be removed or emptied.

The Route handler copies the Gateway's prefix onto the Route **in memory** (`Route.Runtime.NamePrefix`, set with
`SetNamePrefix`, excluded from the API by `json:"-"`) before reconcile and delete. The in-memory state of a Route
(prefix, stored IDs, previous properties) lives in one exported `Runtime` field: unexported fields would make
`equality.Semantic.DeepEqual` panic, which `controllerutil.CreateOrUpdate` uses on Routes (admin operator). `RealmRoute.GetNamePrefix()` returns that value, so discovery, the Kong client and the plugins
are unchanged. The prefix must not be stored in the Kong client: `kongutil` caches clients per admin URL, and
Gateways of several environments that point to the same Kong share one client.

The effective prefix is recorded in `status.properties["namePrefix"]` (only when set). The Route controller watches
Gateways and requeues the Routes that reference a Gateway (field index `spec.gatewayRef` = `namespace/name`) when
its `namePrefix` changes; create, delete and other Gateway updates do not requeue Routes.

Helpers on `Route`: `KongName()` / `GetKongName()` (`prefix + "--" + Name`, or `Name` without prefix),
`GetNamePrefix()`, `GetStoredIds()` / `CaptureStoredIds()`, `GetRouteId()`, `SetLegacyName()`. `StoredIds` lives in
`gateway/api/v1` because the separate `gateway/api` module cannot import the Kong client; the client uses an alias.

## Names and tags

For a Route `R` in environment `E` with `namePrefix` `P`:

| Kong entity | Name | Tags |
|---|---|---|
| Route | `P--R` | today's tags + `realm--P` + preserved `ei__telekom__de--*` |
| Service | `P--R` | today's tags + `realm--P` + preserved `ei__telekom__de--*` |
| Upstream (CB) | `P--R` | `env--E`, `upstream--P--R`, `route--R`, `realm--P` |
| Target (CB) | n/a | `env--E`, `targets--P--R`, `route--R`, `realm--P` |
| Route-bound plugins | n/a | today's tags + `realm--P`. `realm` is **not** an ownership namespace. |

For circuit breaker routes, the service host is `P--R`, the name of the upstream.

**Tag preservation (D9):** when a route or service is written, any tag on the Kong entity that starts with
`ei__telekom__de--` is kept and merged with the desired tags. All other tags stay fully managed as today, so
removing an `additionalTags` entry still takes effect. The equality check uses the same merged set, so an adopted
entity is not rewritten on every reconcile.

## Legacy (Java) naming

The analysis is based on Kong exports of routes written by the Java operator (realms `default`, `bond`,
`default-ai`, and the event routes publish, callback, sse and voyager). These exports are not part of the repository;
the tests use synthetic fixtures with the same structure under `gateway/pkg/kong/legacy/testdata/java-routes`.

- The Java route and its service have the same name: `<javaRealm>.<baseName>`. In Java realm `default` the name is just `<baseName>`.
- AI routes live in a Java realm `<realm>-ai`, e.g. `default-ai.ai-gateway--dhrani-bo-radio-mcp-v1`.
- Java route and service carry `ei__telekom__de--realm---<javaRealm>`. They do not carry their own name as a tag.
- Java plugins carry `[ei__telekom__de--realm---<javaRealm>, <pluginName>, <javaName>]` and are bound to the
  route by ID.
- Java routes have no circuit-breaker upstream, so upstreams have no Java candidates. Most Java services point to
  `localhost:8080/proxy`; the voyager service points to `voyager.qa-horizon` instead.
- For API and AI routes, `baseName` is the same as the CP `metadata.name`. Event routes use different base names:

| CP `metadata.name` | Java `baseName` |
|---|---|
| `publish` | `horizon-events` |
| `callback--<zone>` | `horizon-callback-<zone>` |
| `voyager--<zone>` | `horizon-voyager-<zone>` |
| `sse--<eventTypeName>` | `horizon-sse--<eventTypeName>` |

The CP `publish` route also has the path `/horizon/publish/v1`, which no Java route has. The normal reconcile adds
that path after adoption.

### Config file (D12)

The operator flag `--legacy-naming-config=<path>` is optional. The file is mounted from a ConfigMap.

- If the flag is not set, there are no Java candidates.
- If the flag points to a missing or invalid file (YAML or regex error), the operator fails at startup.

```yaml
# Java operator naming, used only to find legacy Kong entities.
defaultRealm: default        # Java realm whose names carry no realm prefix
separator: "."               # javaName = <javaRealm><separator><baseName>
realmSuffixes: ["", "-ai"]   # Java realms tried for a Route with namePrefix P: P + suffix
names:                       # first matching rule wins; no match → baseName = metadata.name
  - selector: { cp.ei.telekom.de/domain: event }
    match: '^publish$'
    replace: 'horizon-events'
  - selector: { cp.ei.telekom.de/domain: event }
    match: '^callback--(.+)$'
    replace: 'horizon-callback-$1'
  - selector: { cp.ei.telekom.de/domain: event }
    match: '^voyager--(.+)$'
    replace: 'horizon-voyager-$1'
  - selector: { cp.ei.telekom.de/domain: event }
    match: '^sse--(.+)$'
    replace: 'horizon-sse--$1'
```

`selector` is matched against the Route's labels: every key must be present with the same value (also for an empty value). It stops an API Route whose `metadata.name` is `publish` from
mapping to the Java event route `horizon-events`. That event route carries the same realm tag, so the ownership
check alone would not catch it.

### Migrator

```go
// Package legacy (gateway/pkg/kong/legacy) knows how earlier operators named Kong entities.
// It has no dependency on the Kong client; NameScheme (pkg/kong/naming) consumes it.
type Migrator interface {
    // Candidates returns the legacy Kong names under which the route may exist, in lookup order.
    Candidates(route Route) []Candidate
}

type Route struct {
    Name       string            // metadata.name
    NamePrefix string            // effective prefix, from Gateway.spec.namePrefix
    Labels     map[string]string // metadata.labels
}

type Candidate struct {
    Name     string // Kong name of the route and service
    OwnerTag string // tag that proves ownership, e.g. "ei__telekom__de--realm---bond"
}
```

- `NewMigrator(*Config)` builds a Migrator from a config. `LoadMigrator(path)` reads the file strictly and validates
  the regexes; an empty path gives `legacy.None()`, a Migrator without candidates.
- For each suffix in `realmSuffixes`, `javaRealm = P + suffix`, and the name is `baseName` if `javaRealm ==
  defaultRealm`, otherwise `javaRealm + separator + baseName`. The Migrator returns all candidates.
- `naming.NameScheme` turns them into lookup names: the desired name `P--R`, the pre-migration name `R` and the
  remaining candidates without duplicates. A candidate named exactly `R` (Java realm `default`) is not tried twice;
  its owner tag is attached to the `R` step instead (`PreMigrationOwnerTags`).
- The Migrator is created once in `cmd/main.go` and injected into the Kong client through `kongutil`.

## Discovery

**Only when `namePrefix` is set (D3).** The route is looked up first, then the service, then the upstream. Each
lookup stops at the first step that finds an entity.

### Route

1. **Stored ID** `status.properties["routeId"]`, captured at the start of `Builder.Build`. If not found (404), go
   to the next step.
2. **New name** `P--R`. Rejected (`BlockedError`, skipped on delete) if it carries an `env--` tag other than `E` or
   a `route--` tag other than `R`, e.g. the entities of an unprefixed Route `bond--orders` for a Route `orders` with
   `P=bond`. Entities without CP tags are adopted.
3. **CP tags** `env--E,route--R`: all pages, filtered to entities of the expected kind. This finds the route after a
   `namePrefix` change (not after a removal, see D15). More than one match → `BlockedError`.
4. **Pre-migration name** `R`. The route must carry `env--E` **and** `route--R`, or the owner tag of a Java
   candidate named `R` (Java realm `default`); otherwise `BlockedError`.
5. **Java candidates** from the `NameScheme`, in order. The route must carry the candidate's `OwnerTag`, or
   `env--E` + `route--R`; otherwise `BlockedError`.
6. **Not found**: create the route as `P--R`.

Steps 1 and 3 adopt the route without an ownership check, since it is ours by construction. A pre-migration CP route
carries the CP tags, so in practice it is already found at step 3.

| Kong route | Found by | Owned? |
|---|---|---|
| `eni-foo-v1` with `env--bond`, `route--eni-foo-v1` | Route in `bond`, step 3 (or 4) | yes |
| `eni-foo-v1` with `ei__telekom__de--realm---default` | Route with `P=default`, step 4 (Java `default` owner tag) | yes |
| `eni-foo-v1` with `ei__telekom__de--realm---default` | Route with `P=bond`, step 4 | no → `BlockedError` |
| `bond.eni-echo-v3` with `ei__telekom__de--realm---bond` | Route with `P=bond`, step 5 | yes |
| `default-ai.ai-gateway--x` with `ei__telekom__de--realm---default-ai` | Route with `P=default`, step 5 | yes |

**Conflict (D10).** When the found route is about to be renamed (its name is not `P--R`), every **other**
candidate name is checked as well: `P--R`, `R` and the Java candidates. If one of them holds a different route we
own → `BlockedError`. If `P--R` is held by an entity that is **not** ours (e.g. tags of another environment), the
rename would fail with a Kong `409` → `BlockedError` naming the kind, the taken name, its ID and its `env--`/`route--`
tags; a delete skips it. This runs for route, service and upstream before any write, and reuses lookups already
made in the chain. Once the route is called `P--R`, this check is skipped, so a migrated route costs no extra
lookups.

### Service

1. Stored ID `status.properties["serviceId"]`.
2. The service the discovered route points to (`route.service.id`).
3. New name, then CP tags, then pre-migration name `R`, then Java candidates, with the same ownership checks as for
   the route.
4. Not found: create the service as `P--R`.

**Sharing.** A service found at step 2 or under `R` or a Java name, and not yet named `P--R`, is renamed only if no
other Kong route uses it (`GET /services/{id}/routes`). Otherwise → `BlockedError` naming the other route; a delete
skips the service. Once the service is called `P--R`, this check is skipped.

### Upstream (circuit breaker only)

1. Stored ID `status.properties["upstreamId"]`.
2. New name, then CP tags, then pre-migration name `R` (CP tags only). There are no Java candidates.

3. Not found: create the upstream as `P--R`.

Disabling the circuit breaker **or removing the `circuitBreaker` block** deletes the upstream found by this lookup
(delete mode: foreign upstreams are kept) and clears `upstreamId` / `targetsId`. The cleanup runs if an upstream or
target ID is stored or the Route has a `namePrefix`, so a realm Route whose status lost the IDs still finds its
upstream; this costs up to three lookups per reconcile of a realm Route without circuit breaker. Targets are
addressed by the upstream ID, so they work during a rename.

### Write

- An entity that was found is written with `PUT /{entity}/{id}`, using the full desired body: new name, merged tags
  and everything else. Creating still uses `PUT /{entity}/{name}`.
- The existing `reconcile` diff stays in place, so nothing is written once the entity is up to date.
- Kong answers some creates with `201` (e.g. targets) while the generated client only parses `JSON200`.
  `readOne`/`writeOne` decode the raw body when the typed field is empty.
- Order: look up the route (incl. conflict check) → service (incl. sharing check) → upstream; then write the
  upstream and target → service → route; then delete a removed upstream. The circuit breaker feature only hands
  the upstream to write (`CircuitBreakerUpstream`) or to remove (`UpstreamRemoval`) to `CreateOrReplaceRoute`, so a
  blocked lookup writes nothing and the service never points to a deleted upstream.

**Known risk:** renaming the upstream before the service host is updated leaves a window of a few
seconds within one reconcile in which the service host points to the old upstream name.

### Delete

Route, service and upstream go through the same lookup without creating anything, and are deleted **by ID**.
Delete never blocks, so a Route whose reconcile is blocked can still be removed:

- An entity that fails the ownership check is not ours: it is skipped (not deleted), logged, and the lookup
  continues with the next step. More than one CP-tag match is skipped the same way.
- The conflict check (D10) does not block. Only the entity found by the earliest step (stored ID, `P--R`, CP tags,
  `R`, Java candidates) is deleted; other owned duplicates are logged and left in place. At most one entity per
  kind is deleted.
- A shared service (see Service) is skipped.
- If nothing owned is found, the delete succeeds without deleting anything.

The reconcile keeps blocking in all of these cases.

### Status (D13)

Written to `status.properties`: `routeName` (Kong name, for every Route, also without prefix), `routeId`,
`serviceId`, `upstreamId`, `targetsId`, and `legacyName` if the route was adopted under `R` or a Java name.
`legacyName` is kept on later reconciles.

`Builder.Build` rebuilds the properties instead of resetting them midway:

| Phase | Behaviour |
|---|---|
| Start (`StartPropertyRebuild`) | Old properties are kept aside; stored IDs are captured. |
| During the build | Writes go to fresh properties; reads fall back to the old value until the key is written. |
| Success (`FinishPropertyRebuild(true)`) | Keys not written again, or written empty, are dropped. |
| Failure (`FinishPropertyRebuild(false)`) | Old properties stay, with this build's writes on top. |

This keeps the IDs written by features (circuit-breaker `upstreamId`/`targetsId`) and still drops IDs of plugins
that are no longer used.

## Plugins

- `CustomPlugin.GetRoute()` keeps returning `metadata.name`, which feeds the `route--` ownership tag.
- Binding and parent lookup use the Kong **route ID** that `CreateOrReplaceRoute` has just written: the desired
  body gets `route: {id: …}`, and `resolveParentID` and the cleanup scope use the ID. This includes
  `RateLimitPluginFromConsumeRoute`, which now takes the bound Route as second argument.
- **Discovery (D11):**
  1. Stored ID. The plugin must carry exactly the expected ownership tags, or be a legacy plugin in its slot
     (see 3). Anything else is an error.
  2. CP ownership tags. A foreign or ambiguous match is an error.
  3. **Slot lookup**: list all pages of `GET /routes/{routeId}/plugins`. Take the plugin with the same name, no
     service binding and the same consumer binding (or none). Kong allows only one plugin per slot, so there is at
     most one.
     - If it carries any `ei__telekom__de--realm---*` tag, and any ownership tags it already has are a subset of
       the expected ones, adopt it under its existing ID.
     - Any other occupant → `BlockedError`.
- The slot lookup replaces `LegacyPluginTag` and `legacyPluginTags`, also for realm `default`. It applies to every
  route-bound plugin, whether or not `namePrefix` is set.
- Global and consumer-only plugins keep their stored-ID and ownership-tag lookup, with no legacy fallback.
- Cleanup stays unchanged. It only deletes plugins whose CP tags prove ownership.

## Callers

Only the admin operator sets the prefix: when it creates or updates a Gateway it writes
`Gateway.spec.namePrefix = zone.Status.RealmName`. If the realm name is empty, an existing prefix is kept and no
new one is set. The api, agentic, event and admin route builders do not change.

Example: one Gateway `aws-standard` in environment `cit2` serves the `api` and `ai` presets. Its prefix is `cit2`:

| Route | Kong name | Java legacy name found |
|---|---|---|
| `eni-foo-v1` (api) | `cit2--eni-foo-v1` | `cit2.eni-foo-v1` (realm tag `cit2`) |
| `ai-gateway--mcp-v1` (agentic) | `cit2--ai-gateway--mcp-v1` | `cit2-ai.ai-gateway--mcp-v1` (realm tag `cit2-ai`) |
| `dp1-standard--zone-health` (admin) | `cit2--dp1-standard--zone-health` | pre-migration CP name, via CP tags |

## Interface changes (approved as a first draft)

- `KongAdminApi`: add `ListRouteWithResponse`, `ListServiceWithResponse`, `ListUpstreamWithResponse` and
  `ListPluginsForRouteWithResponse`. The generated client already implements them; the mocks must be regenerated.
- New optional interface `RealmRoute`. Implementations of `client.CustomRoute` may implement it, and the Kong client
  checks for it with a type assertion. `*gatewayv1.Route` implements it:
  ```go
  type RealmRoute interface {
      GetNamePrefix() string
      GetKongName() string
      GetLabels() map[string]string
      GetStoredIds() StoredIds // routeId, serviceId, upstreamId, captured at the start of Build
  }
  ```
- New optional plugin interface `RouteIdBinder { GetRouteId() string }`. If it returns an ID, the plugin is bound
  and resolved by that ID. Otherwise the route is resolved by name, as today.
- `LegacyNameRecorder { SetLegacyName(string) }` (route): lets the client record `legacyName`.
- `RealmPlugin { GetNamePrefix() string }` (route-bound plugin): adds the `realm--P` tag.
- `NewKongClient` gets functional options, e.g. `WithMigrator(legacy.Migrator)`. Existing calls keep compiling.
- `CustomRoute` and `CustomPlugin` themselves do not change.

## Architecture

The work is split into layers, each with one job: naming → discovery → write. `reconcile()` itself stays unchanged.

```
          naming (pure)            discovery (I/O)                 write (unchanged)
┌──────────────────────┐   ┌──────────────────────────────┐   ┌─────────────────────┐
│ NameScheme           │──▶│ Locator chain                │──▶│ reconcile()         │
│  desired name P--R   │   │  byID → byName → byTags →    │   │  Get/Project/Write  │
│  legacy candidates ◀─┼─┐ │  byCandidates (+ ownership)  │   │  write by ID        │
└──────────────────────┘ │ └──────────────────────────────┘   └─────────────────────┘
        ▲                │              ▲
  Migrator (port)        │      Ownership (specification)
  config file (adapter) ─┘      CPTags(env,route) OR Tag(ei__…realm---X)
```

1. **Naming** (`pkg/kong/naming`): pure functions with no I/O.
   - `NameScheme` works out the desired Kong name and the ordered candidate names from the Route and a
     `legacy.Migrator`.
   - Tested with plain tables built from the YAML snapshots.
2. **Legacy** (`pkg/kong/legacy`): only the knowledge of how the Java operator named entities.
   - The `Migrator` port and its config-file adapter (`LoadMigrator`).
   - No dependency on the Kong client.
3. **Discovery** (`pkg/kong/client/discovery.go`): a Chain of Responsibility, made of generic `locator[Resp]` steps.
   - Steps: `byID → byParent (service only) → byName(P--R) → byTags(env,route) → byName(R, owned) → byCandidates(owned)`.
   - A `discoveryPolicy` decides what a foreign entity, several tag matches or a duplicate mean: reconcile blocks,
     delete skips (D16).
   - Each step returns `(current, origin, err)`. `nil` means "try the next step".
   - Ownership is a small set of tag predicates (`ownership.go`, e.g. `AnyOf(CPTags(env, route), HasTag(owner))`).
   - The conflict guard (D10) runs after the chain, and only when a rename is pending.
   - Route, service and upstream share the chain through small per-kind adapters (`kinds.go`): get by ID or name,
     list by tags, and read the ID, name and tags.
4. **Write** (`pkg/kong/client/located_entity.go`): a decorator `locatedEntity[Req, Resp]` around the existing
   `routeEntity`, `serviceEntity` and `upstreamEntity`.
   - `Get` runs the chain. `Write` uses `PUT /{id}` when an entity was found.
   - Merging the legacy tags (D9) is a projection and `comparer` decorator.
   - If `namePrefix` is empty, the plain entity is used as today, so there are no `if prefix != ""` branches
     spread through the code.
5. **Plugins** (`plugin_discovery.go`): refactored onto the same chain style, `byID → byOwnershipTags → bySlot(routeID)`.

Removing the migration later only means removing the `byName(R)` and `byCandidates` steps, or configuring no
Migrator. Nothing else changes.

## Implementation plan

**Phase 1: gateway module (done)**

1. API: add `RouteSpec.NamePrefix` and the `Route` methods `KongName()`, `GetNamePrefix()` and `GetStoredIds()`.
   Run `make build` to regenerate the CRDs and deepcopy code.
2. Legacy package `pkg/kong/legacy`: config types, `LoadMigrator(path)`, `NewMigrator`, `Candidates`, validation,
   unit tests. Add the flag `--legacy-naming-config` to `cmd/main.go` and a sample ConfigMap under
   `config/samples/`.
3. Status: `Builder.Build` captures the stored IDs and rebuilds the properties (see Status).
4. Kong client discovery (`discovery.go`, `ownership.go`, `kinds.go`, `located_entity.go`): steps for route,
   service and upstream, ownership checks, conflict check, reconcile/delete policy. Extend `KongAdminApi` and
   regenerate the mocks.
5. Writes by ID with merged legacy tags and the `realm--` tag. The circuit breaker feature takes its names from
   `KongName()`.
6. Delete through discovery, by ID.
7. Plugins: binding by route ID (`RouteIdBinder`), slot lookup replacing the legacy tag lookup, cleanup scope by
   route ID, `realm--` tag on route-bound plugins.
8. Tests (Ginkgo/Gomega):
   - Kong client unit tests for every discovery step, every ownership outcome, the conflict check, renaming by ID,
     tag merging, deletion through each step, and slot adoption.
   - A regression test that an empty `namePrefix` leaves names and tags unchanged.
   - Migrator tests covering every synthetic Java route fixture in `gateway/pkg/kong/legacy/testdata/java-routes`.
   - envtest for the Route controller.
   - Live test on pr-805 (local operator against the vCluster): pre-migration CP route, Java `default`, `bond`,
     `-ai` and event routes adopted; foreign route and duplicates blocked on reconcile and skipped on delete;
     circuit breaker enable/disable; delete of an adopted Java route.
   - Import test on pr-805 with an export of 100 Java routes from a shared QA Kong: 99 CRs
     in 5 realms (incl. 34 event routes) adopted in about 40s; all IDs, legacy tags and unmanaged plugins
     (`rate-limiting-merged`) kept; no duplicates; second reconcile without Kong writes. A Java write-back by ID is
     healed on the next reconcile; a Java re-create by old name is not detected (accepted, see D17).

**Phase 1b: prefix moves from Route to Gateway**

Done.

9. CRDs: remove `Route.spec.namePrefix` (not released), add `Gateway.spec.namePrefix` with a CEL "no removal" rule.
10. gateway: the Route handler sets the effective prefix from the Gateway on the Route in memory (reconcile and
    delete); `status.properties["namePrefix"]`; the Route controller watches Gateways (index on `spec.gatewayRef`).
    Tests move from Route spec to Gateway spec.
11. admin: set `Gateway.spec.namePrefix = zone.Status.RealmName` (keep an existing value when empty), with tests.
    `populateRealmName` runs before `reconcileGateways`, and `Environment.GetRealmName()` falls back to the
    environment name, so the prefix is set on the first reconcile.
12. Docs: spec, `gateway.mdx`, `gateway-realm-migration.mdx`, admin architecture page.

**Phase 2: rollout**

13. Deploy the legacy naming ConfigMap and the `--legacy-naming-config` flag; admin-journey docs for both, and the
    rollout order (stop the Java operator, deploy the config, roll out admin).

## Out of scope

- Kong consumer name collisions across virtual environments.
- Removing legacy `ei__telekom__de--*` tags, and deleting legacy or duplicate entities automatically.
