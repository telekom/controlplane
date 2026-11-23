<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0014: Mask secrets in change records

- Status: Accepted
- Date: 2026-11-20
- Related: [ADR-0012](0012-shared-changeset-for-all-backend-operators.md), [ADR-0013](0013-record-last-applied-changes.md)

## Context

The change pipeline shows the desired and current value of every difference in `status.migration`, in `status.lastAppliedChanges`, in events and in the log ([ADR-0012](0012-shared-changeset-for-all-backend-operators.md), [ADR-0013](0013-record-last-applied-changes.md)). Some backend entities carry secrets: Keycloak client secrets, the Redis password of the Kong rate-limiting plugin, Horizon credentials. Some secrets are not plain fields: the Gateway operator passes client secrets and basic-auth passwords to Kong inside the base64-encoded `jumper_config` entry of the request-transformer plugin. A diff of such an entity would write the secret into the status, where everyone who may read the object sees it, and into logs.

## Decision

- Each entity adapter declares its secrets with a masker. The shared package offers two:
  - **Paths:** the values at JSON paths, such as `$.config.redis_password` or `$.secret`.
  - **Entries:** `key:value` strings in a list whose key matches, such as the `jumper_config` entries under `$.config.*.headers`. The key stays visible; only the value is masked.
- Before the diff, the change pipeline replaces each secret value, on both sides, with `sha256:` and the first 16 hex digits of the SHA-256 of the value. The same value gives the same replacement, so a changed secret is still a difference and changes the plan hash.
- Masking only applies to the change records. Whether an entity is written is still decided on the unmasked values.
- Masking happens in the shared pipeline, not in each adapter, so no record, event or log entry can contain the clear value.
- A test per operator checks that no secret field of its entities appears in clear in a plan.

## Consequences

- Reviewers see that a secret changes, not its value.
- An adapter that forgets to declare a secret leaks it. The per-operator test and code review must catch this.
- A masked `jumper_config` shows that it changed, not which part. Decoding it for the review is a possible follow-up.
- Short or guessable secrets could be found from the hash. The hash is cut to 16 hex digits and only shown to those who may read the object, which is accepted.

## Alternatives considered

- **Leave secret fields out of the diff.** Rejected: a changed secret would not be visible to the reviewer, and the hash would not cover it.
- **Show only `<changed>`.** Rejected: two reconciles could not tell whether the same secret change is still pending, so the plan hash would not be stable.
