---
sidebar_position: 1
slug: /overview
---

# Control Plane

As part of the Open Telekom Integration Platform, the Control Plane is a cloud-agnostic orchestration platform that enables platform engineers to offer self-service API management, configurable approval workflows, and seamless multi-cloud meshing — all from a single, unified interface.

## What is the Control Plane?

The Control Plane acts as the central coordination layer between your teams and the infrastructure they depend on. Rather than managing individual cloud services, gateways, or identity providers directly, teams interact with the Control Plane through a declarative interface. The platform takes care of translating those declarations into the correct configurations across all connected environments.

Key capabilities include:

- **Self-service API management** — Users can register, publish, and subscribe to APIs on their own through the declarative Rover workflow, without needing direct access to the underlying infrastructure.
- **Approval workflows** — API and event subscriptions can be gated by configurable approval strategies (automatic, simple, or four-eyes), giving platform teams control over who accesses what.
- **Multi-cloud meshing** — Services and events are routed across different cloud environments and zones, enabling communication between workloads regardless of where they run.
- **Cloud-agnostic architecture** — The platform abstracts away the specifics of the underlying gateway and identity provider technology, so it can be deployed on any cloud provider or on-premises environment.

## Who is it for?

The Control Plane serves two primary audiences:

- **Platform engineers** set up and manage the platform itself — defining environments, zones, and the rules that govern how APIs and events are shared across teams and cloud boundaries.
- **Application teams** use the platform in self-service to expose their APIs, subscribe to APIs offered by other teams, publish and consume events, and manage their own applications — all through a consistent, declarative workflow.

## Next steps

- [Components](./components.md) — See all the building blocks at a glance
- [Admin Journey](../admin-journey/installation.md) — Install and configure the Control Plane
- [User Journey](../user-journey/onboarding.md) — Start using the platform as an application team
- [Architecture](../architecture/overview.md) — Understand the system design
