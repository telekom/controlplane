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
- **Approval resources** — Approvals that gate listener activation based on provider consent.

Spectre operates as a peer to the Event domain: both create PubSub resources (Publishers and Subscribers), while EventConfig remains the sole creator of EventStores. This shared-adapter model was approved in the July 2025 architectural decision.

> [!NOTE]
> For a detailed architecture diagram, see [docs](./docs/spectre-domain-architecture.md).

## Usage

Users define listeners in their Rover file under `spec.listeners`. The Rover operator creates `Listener` CRs, and the Spectre operator reconciles them into the downstream resources described above.

Key constraints:

- **Pass-through and failover listeners** are currently rejected.
- **SSE delivery** is local-zone-only until cross-zone proxy routes are implemented.
- **Callback traffic** is Gateway-mediated via the zone's `EventConfig.Status.CallbackURL`.
- **Shared generic Publisher** — a single Publisher per application event type may be referenced by multiple Listeners; it is not exclusively owned by any one Listener.

## References

- Peer Domain: [Event Documentation](../event/README.md)
- Runtime Adapter: [PubSub Documentation](../pubsub/README.md)
- Runtime Component: [Horizon Documentation](https://github.com/telekom/pubsub-horizon)
