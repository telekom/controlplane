<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0008: Approve Kong changes by generation

- Status: Superseded by [ADR-0010](0010-approval-by-diff-hash.md)
- Date: 2026-11-20
- Related: [ADR-0007](0007-migration-mode-states.md)

## Context

Blocked changes need a way to be approved from outside the operator. The first implementation approved them with the annotation `gateway.cp.ei.telekom.de/migration-approved-generation` on the Route or Consumer. Its value was the object's `metadata.generation`.

## Decision

An approval covered every Kong change of the object as long as its generation stayed the same. A spec change raised the generation and needed a new approval. Changing the annotation started a reconcile.

## Consequences

Reviewing the flow showed three problems:

- The full differences were only in the operator log.
- The operator stopped at the first blocked entity, so only its differences could be reviewed. The approval also let the later entities of the Route through without review.
- A change of Kong by the Java operator after the approval was overwritten without review, because the generation did not change.

These led to [ADR-0009](0009-plan-kong-changes-as-one-bundle.md), [ADR-0010](0010-approval-by-diff-hash.md) and [ADR-0011](0011-pending-changes-in-status.md).

## Alternatives considered

- **Approve by a hash of the differences.** Not chosen at first because the full set of differences was not known before the first write. Adopted in [ADR-0010](0010-approval-by-diff-hash.md) once the changes were planned as a bundle.
