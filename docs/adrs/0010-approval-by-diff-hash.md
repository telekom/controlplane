<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0010: Approve Kong changes by the hash of the reviewed changes

- Status: Accepted
- Date: 2026-11-20
- Related: Supersedes [ADR-0008](0008-approval-by-generation.md); [ADR-0009](0009-plan-kong-changes-as-one-bundle.md)

## Context

An approval by generation ([ADR-0008](0008-approval-by-generation.md)) also approved changes nobody reviewed, for example a later change of Kong by the Java operator. With the bundle plan of [ADR-0009](0009-plan-kong-changes-as-one-bundle.md), the full set of changes is known before anything is written.

## Decision

- The plan has a hash: the first 16 hex digits of the SHA-256 of all recorded changes, including every value and whether it is ignored.
- The annotation `gateway.cp.ei.telekom.de/migration-approved-hash` approves the bundle with that hash. The Blocked message and `status.migration.hash` name it.
- On the next reconcile, the operator plans again and writes only if the new hash equals the approved one. Changing the annotation starts a reconcile.
- The operator does not remove the annotation.

## Consequences

- An approval covers exactly what was reviewed. Any other change, whether from the Route, from Kong or from a changed ignore list, has a new hash and is blocked again.
- A Route whose Kong state changes often may need several approvals.
- The plan must be deterministic: same changes in the same order and with the same placeholder IDs.

## Alternatives considered

- **Remove the annotation after use.** Rejected: an unchanged hash is safe to apply again, and the operator would need write access to the object's metadata.
- **List the approved hashes.** Not needed: only the pending bundle can be approved.
