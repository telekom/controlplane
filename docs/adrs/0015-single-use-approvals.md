<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0015: Approvals are single-use

- Status: Proposed
- Date: 2026-11-26
- Related: Supersedes [ADR-0010](0010-approval-by-diff-hash.md); [ADR-0012](0012-shared-changeset-for-all-backend-operators.md), [ADR-0013](0013-record-last-applied-changes.md)

## Context

[ADR-0010](0010-approval-by-diff-hash.md) approves a blocked change by its hash: the annotation `cp.ei.telekom.de/migration-approved-hash` ([ADR-0012](0012-shared-changeset-for-all-backend-operators.md)) holds the hash of the reviewed changes. The hash covers only the content of the changes, and the annotation stays on the object. ADR-0010 accepted this because an unchanged hash is safe to apply again.

That reasoning does not hold for a migration. An approval then stays valid forever. If the same change comes back later, its hash is the same and it is written again without a review. For example, the legacy operator sets the same path on a Kong route again, or someone undoes a fix in the backend. A reviewer approved one change at one point in time, not every future change with the same content.

## Decision

- An approval approves one blocked change, once. Each time a change is blocked, the operator creates a random value for it, and the value to approve depends on it.
- `status.migration` gets these fields:

| Field | Meaning |
| ----- | ------- |
| `hash` | The value to approve: the first 16 hex digits of the SHA-256 of `planHash` and `nonce`. Unchanged for the reviewer: approve with `status.migration.hash`. |
| `planHash` | The hash of the changes as in ADR-0010: the first 16 hex digits of the SHA-256 of all recorded changes, including every value and whether it is ignored. |
| `nonce` | A random value (8 bytes from `crypto/rand`, hex) created when the change is first blocked. |
| `blockedAt` | When the change was first blocked. |

- On every reconcile, the operator plans again. If the change still needs an approval, the operator reuses `nonce` and `blockedAt` from the previous `status.migration` if its `planHash` equals the new one. Otherwise, it creates a new `nonce`.
- The change is written only if the annotation equals the new `hash`. Changing the annotation starts a reconcile.
- After the change is written, `status.migration` is removed and the `nonce` with it. `status.lastAppliedChanges` ([ADR-0013](0013-record-last-applied-changes.md)) records `planHash` as `hash` and the approved value as `approvedHash`.
- The operator does not remove the annotation. A left-over annotation never matches a later blocked change.
- Without an approval, nothing changes: in mode `off`, and in mode `on` for changes whose diffs are all ignored, no `nonce` is created.

## Consequences

- An approval covers what was reviewed, once. A change that comes back after it was written is blocked again and needs a new approval, even if its content is the same.
- As in ADR-0010, any other change, whether from the object, from the backend or from a changed ignore list, has another `planHash` and is blocked again.
- While a change waits for approval, its value stays the same across reconciles, as long as `status.migration` is kept.
- If `status.migration` is lost, for example because writing the status failed or the object was restored without its status, the change gets a new `nonce`. It must be approved again; it is never written without an approval.
- The plan must still be deterministic, so that `planHash` and with it the value to approve stay the same while the change waits.
- The `nonce` is not a secret. Whoever can read the status can compute the value. The approval is protected by who may set the annotation, as before.
- Changes blocked before this ADR have no `nonce`. They get one on the next reconcile and must be approved once more.
- The operator needs no write access to the object's metadata.

## Alternatives considered

- **Keep approvals valid as long as the hash is unchanged ([ADR-0010](0010-approval-by-diff-hash.md)).** Rejected, see Context.
- **Remove the annotation after the change is written.** Rejected:
  - the operator needs write access to the object's metadata, as ADR-0010 already noted;
  - writing the metadata in the reconcile conflicts with the status update that follows it;
  - if that write fails after the backend was changed, the approval stays valid;
  - re-applying the object with the annotation, for example with `kubectl apply`, makes it valid again.
- **Record the used hash in the status and reject it afterwards.** Rejected: the next written change replaces `status.lastAppliedChanges`, so the record is lost and the old approval works again. A change that comes back with the same content also has the same hash and could never be approved again.
- **Let approvals expire after a time.** Rejected: an approval can still be used again within that time, and the result depends on clocks.
