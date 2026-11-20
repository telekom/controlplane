<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0009: Plan the Kong changes of a Route as one bundle

- Status: Accepted
- Date: 2026-11-20
- Related: [ADR-0006](0006-migration-mode-diff-guard.md), [ADR-0008](0008-approval-by-generation.md)

## Context

A Route writes its upstream and targets, service, route and plugins one after the other, and later entities depend on the IDs of earlier ones. Checking each entity on its own showed only the first blocked difference ([ADR-0008](0008-approval-by-generation.md)). It could also write some entities and then block on a later one, leaving the Route half migrated.

## Decision

In Migration Mode, each reconcile of a Route or Consumer runs twice:

1. **Plan.** The operator runs the normal build on a copy of the object. Every create, update and delete is recorded instead of executed. Entities that would be created get placeholder IDs starting with `planned--`, so dependent entities can be planned too. Lookups of a planned entity in Kong are skipped.
2. **Decide.** The recorded changes are decided on as one bundle: allowed, approved or blocked.
3. **Apply.** Only an allowed or approved bundle is written, by running the build again without checks.

| Object | Bundle |
| ------ | ------ |
| Route | Upstream and targets, service, route, plugins, removal of unused plugins |
| Consumer | Consumer, consumer group membership, plugins |
| Deleted Route or Consumer | The deletion of its Kong entities |

## Consequences

- Nothing of a blocked Route is written, and the review shows every change at once.
- Each reconcile in Migration Mode reads Kong twice. Without Migration Mode, nothing changes.
- The plan runs on a copy, so placeholder IDs never reach the status.
- Kong can change between plan and apply within one reconcile. This window is accepted.

## Alternatives considered

- **Compare a snapshot of the Route's entities without running the build.** Rejected: it would duplicate the logic that builds the desired state, and the two could drift apart.
- **Write in a transaction.** Not possible: the Kong Admin API has no transactions.
