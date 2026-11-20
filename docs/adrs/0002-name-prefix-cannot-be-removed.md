<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0002: A name prefix cannot be removed

- Status: Accepted
- Date: 2026-11-20
- Related: PR #816, [ADR-0001](0001-realm-prefixed-kong-entity-names.md)

## Context

Discovery follows a changed prefix through the stored IDs and the ownership tags. Without a prefix, the operator looks Kong entities up by `metadata.name` only. It would not find the entities named `<prefix>--<name>` and would create a second set beside them. Deleting the Route would then leave the prefixed entities and their plugins active.

Review of PR #816 found a second way to lose the prefix: moving a Route to another Gateway without a prefix.

## Decision

- The Gateway CRD rejects removing or emptying a set `namePrefix` with a CEL rule. Changing it is allowed.
- Every Route records the prefix it was built with in `status.properties.namePrefix`.
- A Route built with a prefix whose Gateway has none is Blocked and writes nothing.
- Deleting such a Route uses the recorded prefix, so its entities are found and removed.

## Consequences

- The prefix is effectively permanent per Gateway. The Admin operator keeps an existing prefix when it would otherwise remove it (see [ADR-0003](0003-legacy-gateway-naming-flag.md)).
- Moving a prefixed Route to an unprefixed Gateway needs manual cleanup in Kong.

## Alternatives considered

- **Follow a removed prefix by the stored IDs.** Rejected: the unprefixed code path predates realm naming and ignores stored IDs; changing it would change the behaviour for every Gateway without a prefix.
- **Migrate the Route explicitly when its Gateway changes.** Deferred: moving Routes between Gateways is rare and can be handled manually.
