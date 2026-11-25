<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0006: Migration Mode: compare before writing to Kong

- Status: Accepted
- Date: 2026-11-20
- Related: [Migration Mode](../docs/architecture/gateway-realm-migration.mdx#migration-mode), [ADR-0007](0007-migration-mode-states.md), [ADR-0009](0009-plan-kong-changes-as-one-bundle.md)

## Context

The Java operator still manages the Kong configuration. When the Go Gateway operator takes over, its desired state must not differ from what Java configured, except in ways that are expected (for example the new names and tags of [ADR-0001](0001-realm-prefixed-kong-entity-names.md)). An unexpected difference is a breaking change for API consumers and must not reach Kong unnoticed.

The snapshotter tool can already compare Kong snapshots, but only after the fact.

## Decision

- In Migration Mode, the operator compares every Kong entity it would write with the current one before writing. Both sides are serialized to JSON and normalized the same way as the operator's own equality check: sets sorted, legacy tags merged, unmanaged plugin config left out, and `null`, empty lists and empty objects treated as absent.
- Differences are reported by JSON path, such as `$.tags` or `$.config.minute`. A create or delete is one difference at `$`.
- An ignore list of expected differences lives in a YAML file mounted from a ConfigMap and passed with `--migration-ignore-config`. A rule names an entity kind, optionally a name pattern, and JSON paths; `.*` and `[*]` are wildcards, and a path covers everything below it.
- A difference not on the ignore list blocks the change. The Route or Consumer is Blocked; logs, events and the metrics `gateway_kong_migration_checks_total` and `gateway_kong_migration_diffs_total` report it.
- The mode is switched on for the whole operator with a flag, not per Gateway.

## Consequences

- The switch to the Go operator can be done Route by Route without changing Kong unnoticed.
- Only entities the Go operator writes are compared. Configuration that only Java manages is not reported.
- The ignore list must be maintained while the migration runs.

## Alternatives considered

- **Compare full Kong snapshots with the snapshotter.** Rejected as the only control: it finds differences after they were written and cannot stop them.
- **Enable it per Gateway in the Gateway spec.** Rejected: the migration is an operational phase of the whole operator, and a spec field would outlive it.
- **Diff the raw Kong responses.** Rejected: Kong adds defaults and reorders sets, so every write would differ.
