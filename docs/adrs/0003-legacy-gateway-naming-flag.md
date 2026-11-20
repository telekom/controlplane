<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0003: Legacy gateway naming for the default realm

- Status: Accepted
- Date: 2026-11-20
- Related: PR #816 review, [ADR-0001](0001-realm-prefixed-kong-entity-names.md), [ADR-0002](0002-name-prefix-cannot-be-removed.md)

## Context

With [ADR-0001](0001-realm-prefixed-kong-entity-names.md), every Gateway gets the realm name as prefix, also the realm `default`. Installations whose Kong serves only the `default` realm gain nothing from the rename and should keep their unprefixed names.

Review also found that the effective realm name falls back to the environment name. Environment names may contain `--` or be longer than 63 characters, which the Gateway CRD rejects as prefix. Every Gateway update of such an environment would fail.

## Decision

- The feature flag `FEATURE_LEGACY_GATEWAY_NAMING_ENABLED` (default `false`) leaves Gateways of the realm `default` without a prefix.
- The prefix is `Environment.GetRealmName()`.
- A realm name the CRD rejects as prefix gives no new prefix. The Admin operator logs this.
- In both cases a prefix that is already set is kept, because it cannot be removed ([ADR-0002](0002-name-prefix-cannot-be-removed.md)).

## Consequences

- Turning the flag on after Gateways were prefixed does not rename them back.
- Environments with an invalid realm name keep their unprefixed Kong names and are not protected against collisions.

## Alternatives considered

- **Reject such Environment names in the admission webhook.** Rejected: a check on the name would also run on updates and break existing Environments. The code auditor also preferred a CEL rule, which has the same problem.
- **Normalize the realm name into a valid prefix.** Rejected: a prefix that differs from the realm name is surprising and could collide with another realm.
