<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0007: Migration Mode states off, on and strict

- Status: Accepted
- Date: 2026-11-20
- Related: [ADR-0006](0006-migration-mode-diff-guard.md)

## Context

With [ADR-0006](0006-migration-mode-diff-guard.md), a change whose differences are all on the ignore list is written without review. During the first takeovers, every change should be reviewed by a human, even an expected one.

## Decision

The flag `--migration-mode` has three values:

| Mode | The Kong changes are written if |
| ---- | ------------------------------- |
| `off` (default) | always |
| `on` | every difference is ignored, or the changes are approved |
| `strict` | the changes are approved, even if every difference is ignored |

A reconcile without any Kong change needs no approval in either mode.

## Consequences

- `strict` makes every Kong change a manual step, which is safe but slow. It is meant for the first Routes of a migration.
- An approval mechanism is needed ([ADR-0008](0008-approval-by-generation.md), superseded by [ADR-0010](0010-approval-by-diff-hash.md)).

## Alternatives considered

- **A boolean flag plus a separate "require approval" flag.** Rejected: three states are easier to reason about than two interacting flags.
