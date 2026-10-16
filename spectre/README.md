<!--
SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">Spectre Domain</h1>
</p>

<p align="center">
  The Spectre domain manages application listeners that bridge external API traffic into the Horizon event backbone.
  It is a peer business-logic domain alongside the Event domain, and creates PubSub resources directly.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#usage">Usage</a> •
  <a href="#references">References</a>
</p>

## About

The Spectre domain translates `SpectreApplication` and `Listener` custom resources into the downstream resources that wire traffic through the platform:

- **PubSub resources** — Publishers and Subscribers that register listener event flows in the Horizon runtime.
- **Gateway resources** — RouteListeners that expose listener endpoints on the API Gateway.
- **Approval resources** — two keyed ApprovalRequests per Listener, one decided by the provider and one by the consumer; both must be granted before the listener is activated. A gate is auto-approved when the observer (the SpectreApplication's own Application, i.e. the listening application that requests approval) is in the same team as its decider.

Spectre operates as a peer to the Event domain: both create PubSub resources (Publishers and Subscribers), while EventConfig remains the sole creator of EventStores.

> [!NOTE]
> For a detailed architecture diagram, see [docs](./docs/spectre-domain-architecture.md).

## Usage

Users define listeners in their Rover file under `spec.listeners`. The Rover operator creates `Listener` CRs, and the Spectre operator reconciles them into the downstream resources described above.

Key constraints:

- **Pass-through and failover listeners** are currently rejected.
- **`apiBasePath` must be the exact basePath of the API**, including case. A Listener whose path differs, for example by a trailing slash, is Blocked and its capture drained.
- **Event environments must match.** A Listener is Blocked, and its capture drained, when the realm of the capture or observer zone differs from its EventStore's environment (EventConfig `spec.overwriteEnvironmentName`, else the Control Plane environment). Fix it in `EventConfig.spec.overwriteEnvironmentName` or `Environment.spec.realmName`. The SpectreApplication is Blocked by the observer zone check. An EventStore environment of `default` with another realm only raises an `EnvironmentAliasAssumed` Warning event.
- **Capture placement** — capture runs in the consumer zone, else the provider zone; delivery always runs in the observer (listening application) zone. When the capture zone cannot deliver to the observer zone, the Listener is Blocked with `capture zone cannot deliver to observer`; capture does not fall back to the provider zone.
- **Circuit-breaker routes** are supported; the circuit-breaker upstream keeps the Jumper `/listener` path.
- **SSE delivery** works in local and proxy zones. The SSE Route serves `/horizon/sse/v1/de.telekom.ei.listener.<appId>` (canonical) and the legacy alias `/spectre-sse/<appId>`; the full canonical URL is published in `SpectreApplication.status.sseUrl`. The alias does not cover legacy hostnames/TLS or hash-shortened app IDs of very long team names.
- **Callback traffic** is Gateway-mediated via the zone's `EventConfig.Status.CallbackURL`.
- **Shared generic Publisher** — a single Publisher per application event type may be referenced by multiple Listeners; it is not exclusively owned by any one Listener.

## References

- Peer Domain: [Event Documentation](../event/README.md)
- Runtime Adapter: [PubSub Documentation](../pubsub/README.md)
- Runtime Component: [Horizon Documentation](https://github.com/telekom/pubsub-horizon)
