<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0005: Ownership and sharing guards in Kong entity discovery

- Status: Accepted
- Date: 2026-11-20
- Related: PR #816 review, [ADR-0001](0001-realm-prefixed-kong-entity-names.md)

## Context

Review of PR #816 found two ways in which discovery could act on entities of another Route:

- Before the migration, environments with the same Route name upserted one unprefixed entity and recorded the same Kong ID. A lookup by that stored ID could rename or delete the entity of another environment.
- The check whether another Kong route still uses a service ran only for some discovery steps. A shared service found by stored ID or tags could be changed, and deleting it failed on Kong's foreign key, leaving the finalizer stuck.

## Decision

- An entity found by stored or parent ID whose `env--` or `route--` tag names another environment or Route is skipped, and discovery goes on with the next step. Entities without these tags, such as legacy ones, are still found.
- The sharing check runs before every rename of a service, however it was found, and before every delete. A reconcile of a service already under its final name skips it.

## Consequences

- A stale shared ID does not block a Route forever: discovery goes on, blocks while the other environment still owns the pre-migration name and creates its own entity once it has migrated.
- Deletes call Kong once more per service.

## Alternatives considered

- **Block on an ID claimed by another Route.** Rejected: the shared IDs are left over from before the migration and would block the Route permanently.
- **Check sharing on every reconcile.** Rejected: an extra Kong call per Route and reconcile, for a case that can no longer arise once a service has its final name.
