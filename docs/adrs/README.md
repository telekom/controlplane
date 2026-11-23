<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# Architecture Decision Records

This directory records the architecture decisions of the Control Plane: what was decided, why, and what was rejected. An ADR is never rewritten after it is accepted. A changed decision gets a new ADR that supersedes the old one, so the history stays readable.

## Format

Each ADR is a file `NNNN-short-title.md` with the sections Context, Decision, Consequences and Alternatives considered. Its status is one of Proposed, Accepted, Superseded by ADR-NNNN or Deprecated.

## Index

| ADR | Title | Status |
| --- | ----- | ------ |
| [0001](0001-realm-prefixed-kong-entity-names.md) | Realm-prefixed Kong entity names | Accepted |
| [0002](0002-name-prefix-cannot-be-removed.md) | A name prefix cannot be removed | Accepted |
| [0003](0003-legacy-gateway-naming-flag.md) | Legacy gateway naming for the default realm | Accepted |
| [0004](0004-route-full-name.md) | Name the realm-qualified Route name independently of Kong | Accepted |
| [0005](0005-discovery-ownership-and-sharing-guards.md) | Ownership and sharing guards in Kong entity discovery | Accepted |
| [0006](0006-migration-mode-diff-guard.md) | Migration Mode: compare before writing to Kong | Accepted |
| [0007](0007-migration-mode-states.md) | Migration Mode states off, on and strict | Accepted |
| [0008](0008-approval-by-generation.md) | Approve Kong changes by generation | Superseded by [0010](0010-approval-by-diff-hash.md) |
| [0009](0009-plan-kong-changes-as-one-bundle.md) | Plan the Kong changes of a Route as one bundle | Accepted |
| [0010](0010-approval-by-diff-hash.md) | Approve Kong changes by the hash of the reviewed changes | Accepted |
| [0011](0011-pending-changes-in-status.md) | Show pending Kong changes in a temporary status field | Superseded by [0013](0013-record-last-applied-changes.md) |
| [0012](0012-shared-changeset-for-all-backend-operators.md) | One change pipeline for the gateway, identity and pubsub operators | Accepted |
| [0013](0013-record-last-applied-changes.md) | Record the last applied changes in the status | Accepted |
| [0014](0014-mask-secrets-in-change-records.md) | Mask secrets in change records | Accepted |
