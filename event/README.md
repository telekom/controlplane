
<!--
Copyright 2026 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

<p align="center">
  <h1 align="center">Event Domain</h1>
</p>

<p align="center">
    Event Domain is an optional Feature of the Controlplane that allows users to publish and subscribe to events. 
    It is a domain with focus on business-logic between the Rover (User-Config-Layer) and PubSub (Runtime-Config-Layer) Domain. 
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#usage">Usage</a> •
  <a href="#references">References</a> 
</p>

## About

The Event Domain is responsible for handling the business logic of event publishing and subscribing. It acts as an intermediary between the Rover Domain, which is responsible for user configuration, and the PubSub Domain, which handles runtime configuration. The Event Domain provides a structured way to manage events, ensuring that they are published and subscribed to correctly based on the configurations set by users.

The core functions of the Event Domain include:
- **Approval Management**: Handling the approval process between event publishers and subscribers, ensuring that only authorized entities can subscribe to events.
- **Event Routing (Meshing)**: Managing the routing of events from publishers to subscribers based on the configurations set in the Rover Domain.

> [!NOTE]
> For a detailed architecture diagram, see [docs](./docs/event-domain-architecture.md).

## Usage

As this is an optional feature to the Controlplane, it is not configured by default via the admin domain.
Instead, the administrator needs to explicitly enable the Event Domain and deploy its components (Event and PubSub Operators, CRDs, etc.) to the cluster.
Currently, the only supported runtime implementation is [Horizon](https://github.com/telekom/pubsub-horizon).

The core CR to enable the Event Domain is the `EventConfig` CR, which is used to provide the necessary configuration for the Event Domain to function:

```yaml
apiVersion: event.cp.ei.telekom.de/v1
kind: EventConfig
metadata:
  name: test-env # Name must match the environment name for which this EventConfig is created
  namespace: test-env--aws # Namespace must match the namespace of the Zone CR for this environment
  labels:
    app.kubernetes.io/name: event
    app.kubernetes.io/managed-by: kustomize
    cp.ei.telekom.de/environment: test-env
spec:
  # Zone for which this EventConfig is created. Must match the Zone CR for this environment.
  zone:
    name: aws
    namespace: test-env

  # Mesh configuration for event routing between zones. If fullMesh is true, all zones will be meshed together.
  mesh:
    fullMesh: false
    zoneNames: 
      - aws
      - azure
      - gcp

  # Admin backend (quasar configuration API).
  admin:
    url: https://my-horizon-instance-aws.test.dhei.telekom.de/api/v1/resources

    # Identity Realm for OAuth2 authentication with the admin backend.
    realm:
      name: test-env
      namespace: test-env--aws

  # Internal URL of the SSE backend service (e.g. horizon-tasse).
  # Used as the upstream for the SSE gateway Route created by EventExposure.
  serverSendEventUrl: "https://my-horizon-instance-aws.test.dhei.telekom.de/api/v1/sse"

  # Internal URL of the publish backend service (e.g. horizon-producer).
  # Used as the upstream for the publish gateway Route
  publishEventUrl: "https://my-horizon-instance-aws.test.dhei.telekom.de/api/v1/events"
```

## References

- PubSub Domain: [PubSub Documentation](../pubsub/README.md)
- Rover Domain: [Rover Documentation](../rover/README.md)
