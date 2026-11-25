<!--
SPDX-FileCopyrightText: 2026 Deutsche Telekom AG

SPDX-License-Identifier: CC0-1.0
-->

# ADR-0004: Name the realm-qualified Route name independently of Kong

- Status: Accepted
- Date: 2026-11-20
- Related: PR #816 review

## Context

PR #816 added `Route.KongName()` and `Route.GetKongName()`, which return `<namePrefix>--<metadata.name>`. The two methods did the same, and their names tied the Route API type to one gateway product.

## Decision

A single method `Route.GetFullName()` returns the realm-qualified name. The Kong client's `RealmRoute` interface uses it.

## Consequences

The API type does not mention Kong. Other gateway backends can use the same name.

## Alternatives considered

- **`GetID()`.** Rejected: the Route also tracks Kong IDs (`routeId`, `serviceId`, `upstreamId`), and an `ID` method returning a name would be confused with them.
