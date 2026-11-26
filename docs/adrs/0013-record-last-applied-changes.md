<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0013: Record the last applied changes in the status

- Status: Accepted
- Date: 2026-11-20
- Related: Supersedes [ADR-0011](0011-pending-changes-in-status.md); [ADR-0012](0012-shared-changeset-for-all-backend-operators.md)

## Context

[ADR-0011](0011-pending-changes-in-status.md) shows the pending changes of a blocked Route or Consumer in `status.migration` and removes the field once the changes are written. After that, nothing on the object says what the operator last changed in the backend, when, or why it was allowed. The diff engine was also meant to be removed after the migration.

The diff is useful beyond the migration: it is an audit trail of what each operator did to Kong, Keycloak and Horizon.

## Decision

- The change pipeline ([ADR-0012](0012-shared-changeset-for-all-backend-operators.md)) runs in every mode, including `off`. Migration Mode is only the gate on top of it: with `off`, every bundle is allowed.
- `status.migration` keeps its role: the pending changes, only while they are blocked.
- A new field `status.lastAppliedChanges` records the last bundle that was applied. It is replaced on every applied bundle and stays on the object.

| Field | Meaning |
| ----- | ------- |
| `hash` | Hash of the applied bundle |
| `appliedAt` | Time of the apply |
| `generation` | `metadata.generation` the bundle was built from |
| `mode` | Migration Mode at the time: `off`, `on` or `strict` |
| `decision` | `allowed` or `approved` |
| `approvedHash` | The approved hash, if the bundle was approved |
| `changes` | Kind, name, operation and diffs of every change, as in `status.migration` |
| `truncated` | True if changes or values were cut to fit the size limits |

- A reconcile without any change leaves `lastAppliedChanges` as it is. `appliedAt` is not part of the hash.
- Every applied bundle also emits a Kubernetes Event `ChangesApplied` with the hash, and a log entry with all changes.
- Size limits keep the object small: a value is cut after 1024 characters and at most 100 diffs are stored, in both fields. `lastAppliedChanges` records a created entity by the hash of its body; `status.migration` shows the body, because the reviewer needs to see what would be created. The hash always covers the full bundle.

## Consequences

- `kubectl get <object> -o yaml` shows what the operator last changed, when, under which mode and why it was allowed.
- The diff engine and the status field are permanent. After the migration, only the gate is switched off.
- The record is not a history: each applied bundle replaces the previous one, and the record is lost when the object is recreated. The history is in the events, the operator log and the Kubernetes API audit log.
- Who set the approval annotation is not recorded in the status, because the operator cannot verify it. The Kubernetes API audit log has it.
- The status is written once per applied bundle. It does not change the generation, so it starts no reconcile.

## Alternatives considered

- **Keep `status.migration` temporary and record nothing.** Rejected, see Context.
- **A history of the last N bundles in the status.** Rejected: the status grows with every change, and a history belongs in the log or audit system.
- **A custom resource per applied bundle.** Rejected: many objects, extra RBAC and cleanup for something events and logs already cover.
- **Record the approving user.** Rejected: the operator only sees the annotation, not who set it. A recorded name would look trustworthy without being so.
