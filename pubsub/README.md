<!--
Copyright 2026 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">PubSub Domain</h1>
</p>

<p align="center">
  Publish & Subscribe (PubSub) is a messaging pattern where senders (publishers) send messages to a topic, and receivers (subscribers) receive messages from that topic (event-types)
  It allows for decoupling of components and asynchronous communication. This implementation relies on [Horizon](https://github.com/telekom/pubsub-horizon) as the underlying messaging system.
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#usage">Usage</a> •
  <a href="#references">References</a> 
</p>

## About

The PubSub Domain is an extension (Feature enabled via Flag) to the Controlplane that provides a Publish & Subscribe (PubSub) messaging system. 
It allows for decoupling of components and asynchronous communication. 
It integrates with the Controlplane cores features such as Gateway and Approval to provide a seamless experience for users.

This domain is used as a `Runtime-Config-Layer` for the Controlplane, which means that it is the last layer before the configuration is written to the runtime component (Horizon)

For a more detailed view of the internal workings of the Horizon components, please refer to the [Horizon Documentation](https://github.com/telekom/pubsub-horizon). The pubsub Domain is designed as a thin bridge between the Controlplane and Horizon.

> [!NOTE]
> For a detailed architecture diagram, see [docs](./docs/pubsub-domain-architecture.md).


## Usage

This domain is configured through the Event and Spectre domains — users do not interact with PubSub resources directly.
The Event domain creates Publishers (via EventExposure) and Subscribers (via EventSubscription).
The Spectre domain creates Publishers (via SpectreApplication) and Subscribers (via Listener).
EventStore is created exclusively by EventConfig; Spectre reads it but does not own it.

## References

- Runtime Component: [Horizon Documentation](https://github.com/telekom/pubsub-horizon)
- Event Domain: [Event Documentation](../event/README.md)
- Spectre Domain: [Spectre Documentation](../spectre/README.md)

