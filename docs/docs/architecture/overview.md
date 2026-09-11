---
sidebar_position: 1
---

# Architecture Overview

The Control Plane follows a Kubernetes-native architecture built on the operator pattern. Each domain is implemented as an independent operator that manages its own set of custom resources (CRDs) and reconciles them to the desired state. Domains communicate through shared Kubernetes resources rather than direct API calls, keeping each operator loosely coupled and independently deployable.

## Design Principles

- **Declarative** — Users define the desired state; controllers reconcile it
- **Kubernetes-native** — Built on CRDs, controllers, and standard Kubernetes patterns
- **Domain-driven** — Each operator owns a specific bounded context with clear responsibilities
- **Extensible** — New operators can be added without modifying core components
- **Cloud-agnostic** — Infrastructure abstractions allow deployment on any cloud or on-premises environment
- **Observable** — All components expose metrics and structured logs

## High-Level Architecture

The following diagram shows how the Control Plane is structured — from the user-facing entry points on the left, through the central processing logic, all the way to the runtime configuration that connects to the actual infrastructure.

![High-Level Architecture of the Control Plane](/img/highlevel-architecture.svg)

Two key design approaches shape the architecture of the Control Plane: **Domain-Driven Design** and a **Layer Architecture**. Together, they ensure that the system remains modular, maintainable, and easy to extend.

### Domain-Driven Design

The Control Plane system is divided into **domains**, where each domain has a specific purpose and offers an API so that other domains can interact with it. This approach is inspired by Domain-Driven Design (DDD), which emphasizes clear boundaries and well-defined responsibilities for each part of the system.

Each domain contains two separate modules:

- **Application** — The core logic and controllers that implement the domain's behavior.
- **API** — The interface that any other domain can use to interact with this domain. This keeps domains loosely coupled: they communicate through stable APIs rather than relying on internal implementation details.

### Layer Architecture

In addition to domain boundaries, the Control Plane is organized into **layers**. Each layer fulfills a different purpose in the lifecycle of a user's request as it flows through the system.

#### Admin Layer

The Admin Layer is exclusively used by the administrators of the Control Plane. It provides the tools to configure global components such as environments, zones, and remote organizations (planned feature) — the foundational infrastructure that all other layers depend on.

#### UI Layer

The UI Layer is where users interact with their configured components. Through a federated access model — set up and managed by the Admin Layer — users can view and manage the state of their resources across different environments from a single interface. This layer is powered by the [ControlPlane API & Projector](./controlplane-api.mdx), which provide read-only GraphQL access to the platform state.

#### Customer-Config Layer

The Customer-Config Layer serves as the main entry point to the system for customers. When a user submits a configuration (for example, a Rover file), this layer receives and validates it before passing it along. Most of the input validation happens here, ensuring that only well-formed requests reach the deeper layers of the system.

#### Logic Layer

The Logic Layer is the most central part of the architecture. It is responsible for processing the resources requested by the user — handling tasks such as approval workflows, resolving dependencies between domains, and provisioning the correct runtime configuration. This is where the main business logic lives.

#### Runtime-Config Layer

The Runtime-Config Layer is the final step in the processing chain. It takes the configuration produced by the Logic Layer and applies it to the actual components running on the data plane — such as the API Gateway, the Identity Provider, or the event messaging infrastructure.

## Domain Interaction Model

Domains interact primarily through Kubernetes resources. A higher-level domain creates resources that a lower-level domain reconciles. For example:

1. A user applies a **Rover** file through Rover-CTL or Rover Server
2. The **Rover** operator creates **Application**, **Api**, **ApiExposure**, and **ApiSubscription** resources
3. The **API** operator processes exposures and subscriptions, creating **Approval** resources where needed
4. The **Approval** operator manages the approval workflow and updates the approval status
5. Once approved, the **API** operator creates **Gateway Route** and **ConsumeRoute** resources
6. The **Gateway** operator configures the actual API gateway (e.g., Kong)
7. The **Identity** operator provisions the corresponding authentication clients in the identity provider (e.g., Keycloak)

## Virtual Environments

The Control Plane supports multi-tenancy through **virtual environments**. Each custom resource is assigned to an environment via labels, and operators use a scoped Kubernetes client that automatically filters resources by environment. This allows multiple environments (dev, staging, production) to coexist within a single Kubernetes cluster.

## Reconciliation Pattern

All operators share a common reconciliation pattern provided by the [Common](https://github.com/telekom/controlplane/tree/main/common) library:

1. **Controller** — Watches for changes to custom resources and triggers reconciliation
2. **Handler** — Implements the domain-specific business logic (create/update or delete)
3. **ScopedClient** — A context-aware Kubernetes client that respects virtual environment boundaries
4. **Conditions** — Standardized status conditions (Ready, Processing, Blocked, Done) used across all domains

## Domain Pages

Each domain is described in detail on its own page:

| Domain | Description |
| ------ | ----------- |
| [Admin](./admin.mdx) | Environments, zones, and remote organizations (planned feature) |
| [Organization](./organization.mdx) | Teams, groups, and auto-provisioning |
| [Application](./application.mdx) | Application abstraction and resource provisioning |
| [API](./api.mdx) | API lifecycle — registration, exposure, and subscription |
| [Rover](./rover.mdx) | Declarative user entry point |
| [Approval](./approval.mdx) | Approval workflows and state machines |
| [Notification](./notification.mdx) | Notification delivery and templates |
| [Gateway](./gateway.mdx) | API Gateway configuration |
| [Identity](./identity.mdx) | Identity and access management |
| [Event](./event.mdx) | Event publishing, subscribing, and meshing |
| [PubSub](./pubsub.mdx) | Runtime layer for publish/subscribe messaging |
| [Secret Manager](./secret-manager.mdx) | Centralized secret storage, references, and retrieval |
| [ControlPlane API & Projector](./controlplane-api.mdx) | Read-only external access layer (CQRS) for the UI |
