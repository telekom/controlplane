<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0001: Realm-prefixed Kong entity names

- Status: Accepted
- Date: 2026-11-20
- Related: PR #816, [design: gateway realm](../design/gateway-realm.md), [Kong entity naming and realms](../docs/architecture/gateway.mdx)

## Context

The Gateway operator named every Kong entity of a Route after `Route.metadata.name`. Several virtual environments, such as `bond`, `cit2` and `cit4`, share one Kong. The same Route name in two environments then maps to the same Kong route, service and upstream, and the environments overwrite each other.

Kong also still holds the entities of the legacy Java operator, named `<legacyRealm>.<name>`, and entities the Gateway operator created before this change, named `<name>`. They must be taken over without downtime.

## Decision

- The Kong name of a Route's route, service and circuit-breaker upstream is `<namePrefix>--<metadata.name>`, for every realm including `default`.
- The prefix is set once per Gateway in the optional field `Gateway.spec.namePrefix`. The Admin operator sets it to the realm name of the environment. Routes have no prefix field of their own.
- Existing entities are found and renamed in place, so their Kong IDs stay the same. Discovery tries, in order: the stored ID, the parent entity, the new name, the ownership tags `env--`/`route--`, the pre-migration name and the legacy candidate names.
- The legacy naming (`defaultRealm`, `separator`, `realmSuffixes`) lives in an optional config file mounted from a ConfigMap.
- Plugins are bound to their route by Kong route ID. Adopted entities keep their legacy tags and get the tag `realm--<namePrefix>`.
- Event Routes carry the legacy-compatible base names (for example `horizon-events`) as `metadata.name`.

## Consequences

- Environments sharing one Kong no longer overwrite each other.
- Renaming in place keeps the Kong IDs, so consumers, plugins and metrics referring to them keep working.
- The legacy operator must stop writing a Route before the Gateway operator takes it over. A route it recreates later under its old name is not detected.
- Kong consumer name collisions across environments are out of scope.

## Alternatives considered

- **A prefix field on every Route.** Rejected: the prefix is a property of the Kong instance and realm, not of a single Route, and every operator creating Routes would have to set it.
- **`Route.spec.security.realmName` as prefix.** Rejected: it controls the Last-Mile-Security realm, an unrelated concept.
- **Create new entities and delete the old ones.** Rejected: new Kong IDs would break references, and a failed switch would leave traffic without a route.
