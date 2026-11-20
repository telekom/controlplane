<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0014: Mask secrets in change records

- Status: Proposed
- Date: 2026-11-20
- Related: [ADR-0012](0012-shared-changeset-for-all-backend-operators.md), [ADR-0013](0013-record-last-applied-changes.md)

## Context

The change pipeline shows the desired and current value of every difference in `status.migration`, in `status.lastAppliedChanges`, in events and in the log ([ADR-0012](0012-shared-changeset-for-all-backend-operators.md), [ADR-0013](0013-record-last-applied-changes.md)). Some backend entities carry secrets: Keycloak client secrets, Kong basic-auth and JWT credentials, Horizon credentials. A diff of such an entity would write the secret into the status, where everyone who may read the object sees it, and into logs.

## Decision

- Each entity adapter declares the JSON paths of its secret fields.
- Before the diff, the change pipeline replaces the value at each secret path, on both sides, with `sha256:` and the first 16 hex digits of the SHA-256 of the value. The same value gives the same replacement, so a changed secret is still a difference and changes the plan hash.
- Masking happens in the shared pipeline, not in each adapter, so no record, event or log entry can contain the clear value.
- A test per operator checks that no secret field of its entities appears in clear in a plan.

## Consequences

- Reviewers see that a secret changes, not its value.
- An adapter that forgets to declare a secret path leaks it. The per-operator test and code review must catch this.
- Short or guessable secrets could be found from the hash. The hash is cut to 16 hex digits and only shown to those who may read the object, which is accepted.

## Alternatives considered

- **Leave secret fields out of the diff.** Rejected: a changed secret would not be visible to the reviewer, and the hash would not cover it.
- **Show only `<changed>`.** Rejected: two reconciles could not tell whether the same secret change is still pending, so the plan hash would not be stable.
