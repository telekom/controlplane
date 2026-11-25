<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0011: Show pending Kong changes in a temporary status field

- Status: Accepted
- Date: 2026-11-20
- Related: [ADR-0009](0009-plan-kong-changes-as-one-bundle.md), [ADR-0010](0010-approval-by-diff-hash.md)

## Context

Reviewers had to search the operator log for blocked differences ([ADR-0008](0008-approval-by-generation.md)). Logs need extra access, rotate and are lost on restart. The Blocked condition can only carry a short summary.

## Decision

- A Route or Consumer whose bundle is blocked gets `status.migration` with the hash, the mode and every change: kind, name, operation and the differences with desired and current value as JSON and whether they are ignored.
- The field is temporary: it is removed as soon as the bundle is allowed or approved, or when Migration Mode is off.
- Values longer than 1024 characters are truncated in the status; the hash covers the full value. The log has everything.

## Consequences

- A review is `kubectl get route <name> -o yaml`, without log access.
- The Route and Consumer CRDs carry a field that is only used during the migration. It can be removed together with Migration Mode.
- Large plugin configurations make the status bigger while a change is blocked.

## Alternatives considered

- **Only the log.** Rejected, see Context.
- **A separate custom resource per pending change.** Rejected: more objects, RBAC and cleanup for a temporary need.
- **An annotation with the diff.** Rejected: annotations are size-limited, and the operator should not write the object's metadata.
